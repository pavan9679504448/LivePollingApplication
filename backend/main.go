package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type App struct {
	db     *mongo.Database
	redis  *redis.Client
	origin string
	secure bool
	secret []byte
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	origin := strings.TrimRight(env("APP_ORIGIN", "http://localhost:8080"), "/")
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.User != nil || u.Fragment != "" {
		log.Fatal("APP_ORIGIN must be an origin, such as https://your-app.example")
	}
	secret := os.Getenv("COOKIE_SECRET")
	if len(secret) < 32 {
		log.Fatal("COOKIE_SECRET must contain at least 32 characters. See the setup guide.")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(env("MONGODB_URI", "mongodb://localhost:27017")).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		log.Fatal("MongoDB configuration failed")
	}
	redisOptions, err := redis.ParseURL(env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		log.Fatal("REDIS_URL is invalid")
	}
	rdb := redis.NewClient(redisOptions)
	app := &App{db: client.Database(env("MONGODB_DB", "pulsepoll")), redis: rdb, origin: origin, secure: u.Scheme == "https", secret: []byte(secret)}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err = client.Ping(ctx, nil); err != nil {
		log.Fatal("MongoDB is unavailable; check its connection settings")
	}
	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("Redis is unavailable; check its connection settings")
	}
	if err = app.indexes(ctx); err != nil {
		log.Fatal("Could not create database indexes: ", err)
	}
	cancel()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), app.security)
	var trusted []string
	if value := os.Getenv("TRUSTED_PROXIES"); value != "" {
		for _, item := range strings.Split(value, ",") {
			trusted = append(trusted, strings.TrimSpace(item))
		}
	}
	if err = router.SetTrustedProxies(trusted); err != nil {
		log.Fatal("Invalid TRUSTED_PROXIES")
	}
	router.GET("/healthz", app.health)
	api := router.Group("/api")
	api.POST("/auth/signup", app.limit("auth", 30, 15*time.Minute), app.signup)
	api.POST("/auth/login", app.limit("auth", 30, 15*time.Minute), app.login)
	api.POST("/auth/logout", app.logout)
	api.GET("/auth/me", app.requireUser, func(c *gin.Context) { c.JSON(200, c.MustGet("user")) })
	api.GET("/polls", app.requireUser, app.listPolls)
	api.POST("/polls", app.requireUser, app.limit("create", 20, time.Minute), app.createPoll)
	api.GET("/polls/:id", app.getPoll)
	api.POST("/polls/:id/vote", app.limit("vote", 120, time.Minute), app.vote)
	api.POST("/polls/:id/close", app.requireUser, app.closePoll)
	api.GET("/polls/:id/events", app.events)
	staticDir := env("STATIC_DIR", "../frontend/dist")
	router.Static("/assets", filepath.Join(staticDir, "assets"))
	router.GET("/favicon.svg", func(c *gin.Context) { c.File(filepath.Join(staticDir, "favicon.svg")) })
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != "GET" || strings.HasPrefix(c.Request.URL.Path, "/api/") {
			fail(c, 404, "Page not found.")
			return
		}
		if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
			fail(c, 503, "Frontend build missing. Run npm run build in frontend.")
			return
		}
		c.File(filepath.Join(staticDir, "index.html"))
	})
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 75 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		log.Printf("PulsePoll listening on port %s", env("PORT", "8080"))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = rdb.Close()
	_ = client.Disconnect(shutdownCtx)
}

func (a *App) indexes(ctx context.Context) error {
	if _, err := a.db.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
		return err
	}
	if _, err := a.db.Collection("sessions").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}); err != nil {
		return err
	}
	_, err := a.db.Collection("polls").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "createdAt", Value: -1}}})
	return err
}

func fail(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{"error": message})
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func dbContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 5*time.Second)
}

func (a *App) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if a.db.Client().Ping(ctx, nil) != nil || a.redis.Ping(ctx).Err() != nil {
		fail(c, 503, "A required service is unavailable.")
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (a *App) security(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if a.secure {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.Header("Cache-Control", "no-store")
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
		// Custom header forces a CORS preflight for cross-origin browser writes.
		// No cross-origin access is granted; Origin is checked as a second guard.
		if c.GetHeader("X-PulsePoll-Request") != "1" {
			fail(c, 403, "Request verification failed.")
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" && origin != a.origin {
			fail(c, 403, "This origin is not allowed.")
			return
		}
		if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
			fail(c, 415, "Send JSON data.")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	}
	c.Next()
}

var rateScript = redis.NewScript(`local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]) end; return n`)

func (a *App) limit(kind string, max int64, period time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := dbContext(c)
		defer cancel()
		n, err := rateScript.Run(ctx, a.redis, []string{"pulsepoll:limit:" + kind + ":" + hash(c.ClientIP())}, int(period.Seconds())).Int64()
		if err != nil {
			fail(c, 503, "Live services are temporarily unavailable. Please try again.")
			return
		}
		if n > max {
			c.Header("Retry-After", "60")
			fail(c, 429, "Too many requests. Please wait before trying again.")
			return
		}
		c.Next()
	}
}
