package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (a *App) events(c *gin.Context) {
	id := c.Param("id")
	if !validID(id) {
		fail(c, 404, "Poll not found.")
		return
	}
	visitor, owner, ok := a.identity(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	pubsub := a.redis.Subscribe(ctx, "pulsepoll:poll:"+id)
	defer pubsub.Close()
	// Subscribe before reading the initial snapshot so no vote is lost between
	// taking the snapshot and starting to listen for new votes.
	readyCtx, readyCancel := context.WithTimeout(ctx, 5*time.Second)
	_, err := pubsub.Receive(readyCtx)
	readyCancel()
	if err != nil {
		fail(c, 503, "Live updates are reconnecting.")
		return
	}
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	poll, err := a.readPoll(readCtx, id, visitor, owner)
	readCancel()
	if errors.Is(err, mongo.ErrNoDocuments) {
		fail(c, 404, "Poll not found.")
		return
	}
	if err != nil {
		fail(c, 503, "Could not read results.")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	write := func(p *Poll) bool {
		data, err := json.Marshal(p)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(c.Writer, "id: %d\nevent: results\ndata: %s\n\n", p.Version, data); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	if !write(poll) {
		return
	}
	version := poll.Version
	messages := pubsub.Channel()
	reconcile := time.NewTicker(10 * time.Second)
	defer reconcile.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-messages:
			if !open {
				return
			}
		case <-reconcile.C:
			// Pub/Sub is not durable. Re-read canonical data to recover a missed
			// publish or subscription reconnect; this also keeps the stream alive.
		}
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		next, err := a.readPoll(readCtx, id, visitor, owner)
		cancel()
		if err != nil {
			return
		}
		if next.Version > version {
			if !write(next) {
				return
			}
			version = next.Version
		} else {
			if _, err = fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
