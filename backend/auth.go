package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

func token() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(value)
}
func (a *App) cookie(c *gin.Context, name, value string, seconds int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: seconds, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
}

func (a *App) currentUser(c *gin.Context) (*User, error) {
	raw, err := c.Cookie("pulse_session")
	if err != nil || len(raw) != 64 {
		return nil, nil
	}
	ctx, cancel := dbContext(c)
	defer cancel()
	var session Session
	err = a.db.Collection("sessions").FindOne(ctx, bson.M{"_id": hash(raw), "expiresAt": bson.M{"$gt": time.Now()}}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var user User
	err = a.db.Collection("users").FindOne(ctx, bson.M{"_id": session.UserID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (a *App) requireUser(c *gin.Context) {
	user, err := a.currentUser(c)
	if err != nil {
		fail(c, 503, "Could not verify your account. Please retry.")
		return
	}
	if user == nil {
		fail(c, 401, "Please sign in to continue.")
		return
	}
	c.Set("user", user)
	c.Next()
}

func (a *App) startSession(c *gin.Context, user *User) error {
	raw := token()
	ctx, cancel := dbContext(c)
	defer cancel()
	_, err := a.db.Collection("sessions").InsertOne(ctx, Session{ID: hash(raw), UserID: user.ID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
	if err == nil {
		a.cookie(c, "pulse_session", raw, 7*24*3600)
	}
	return err
}

func (a *App) signup(c *gin.Context) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		fail(c, 400, "Please provide valid signup details.")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	address, err := mail.ParseAddress(input.Email)
	if utf8.RuneCountInString(input.Name) < 2 || utf8.RuneCountInString(input.Name) > 60 || err != nil || address.Address != input.Email || len(input.Email) > 254 {
		fail(c, 400, "Enter a name of 2–60 characters and a valid email.")
		return
	}
	if len(input.Password) < 10 || len(input.Password) > 72 {
		fail(c, 400, "Use a password between 10 and 72 bytes (usually characters).")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		fail(c, 500, "Could not create your account.")
		return
	}
	user := User{ID: token()[:24], Name: input.Name, Email: input.Email, PasswordHash: string(passwordHash)}
	ctx, cancel := dbContext(c)
	defer cancel()
	_, err = a.db.Collection("users").InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		fail(c, 409, "An account with this email already exists. Please sign in.")
		return
	}
	if err != nil {
		fail(c, 503, "Could not save your account. Please retry.")
		return
	}
	if a.startSession(c, &user) != nil {
		fail(c, 503, "Account created. Please sign in to continue.")
		return
	}
	c.JSON(201, user)
}

func (a *App) login(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.Password) > 72 || len(input.Email) > 254 {
		fail(c, 400, "Enter a valid email and password.")
		return
	}
	ctx, cancel := dbContext(c)
	defer cancel()
	var user User
	err := a.db.Collection("users").FindOne(ctx, bson.M{"email": strings.ToLower(strings.TrimSpace(input.Email))}).Decode(&user)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		fail(c, 503, "Sign in is temporarily unavailable.")
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		fail(c, 401, "Email or password is incorrect.")
		return
	}
	if a.startSession(c, &user) != nil {
		fail(c, 503, "Could not start your session. Please retry.")
		return
	}
	c.JSON(200, user)
}

func (a *App) logout(c *gin.Context) {
	if raw, err := c.Cookie("pulse_session"); err == nil {
		ctx, cancel := dbContext(c)
		defer cancel()
		if _, err = a.db.Collection("sessions").DeleteOne(ctx, bson.M{"_id": hash(raw)}); err != nil {
			fail(c, 503, "Could not sign out. Please retry.")
			return
		}
	}
	a.cookie(c, "pulse_session", "", -1)
	c.JSON(200, gin.H{"ok": true})
}

func (a *App) signVisitor(raw string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(raw))
	return hex.EncodeToString(m.Sum(nil))
}

// Guest votes are limited per signed browser cookie, not per human identity.
func (a *App) visitor(c *gin.Context) string {
	value, _ := c.Cookie("pulse_visitor")
	parts := strings.Split(value, ".")
	if len(parts) == 2 && len(parts[0]) == 64 && hmac.Equal([]byte(parts[1]), []byte(a.signVisitor(parts[0]))) {
		return hash("guest:" + parts[0])
	}
	raw := token()
	a.cookie(c, "pulse_visitor", raw+"."+a.signVisitor(raw), 30*24*3600)
	return hash("guest:" + raw)
}
