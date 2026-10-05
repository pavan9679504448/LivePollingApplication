package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func validatePoll(in *CreatePollInput) error {
	in.Question = strings.TrimSpace(in.Question)
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Question) < 5 || utf8.RuneCountInString(in.Question) > 200 {
		return errors.New("Your question must be 5–200 characters.")
	}
	if utf8.RuneCountInString(in.Description) > 400 {
		return errors.New("Keep the description within 400 characters.")
	}
	if len(in.Options) < 2 || len(in.Options) > 6 {
		return errors.New("Add between 2 and 6 options.")
	}
	seen := map[string]bool{}
	for i, item := range in.Options {
		item = strings.TrimSpace(item)
		if item == "" || utf8.RuneCountInString(item) > 80 {
			return errors.New("Each option must contain 1–80 characters.")
		}
		key := strings.ToLower(item)
		if seen[key] {
			return errors.New("Each option must be different.")
		}
		seen[key] = true
		in.Options[i] = item
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 24 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
func (a *App) identity(c *gin.Context) (string, string, bool) {
	guest := a.visitor(c)
	user, err := a.currentUser(c)
	if err != nil {
		fail(c, 503, "Could not load your session.")
		return "", "", false
	}
	if user != nil {
		return guest, user.ID, true
	}
	return guest, "", true
}

func (a *App) readPoll(ctx context.Context, id, visitor, owner string) (*Poll, error) {
	projection := bson.M{"ballots": 0}
	// Fetch just this browser's receipt. Never return other visitors' receipts.
	if visitor != "" {
		projection = bson.M{"question": 1, "description": 1, "options": 1, "open": 1, "total": 1, "version": 1, "createdAt": 1, "ownerId": 1, "ballots." + visitor: 1}
	}
	var poll Poll
	err := a.db.Collection("polls").FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(projection)).Decode(&poll)
	if err != nil {
		return nil, err
	}
	poll.MyChoice = poll.Ballots[visitor]
	poll.IsOwner = owner != "" && owner == poll.OwnerID
	poll.Capacity = maxVotes
	return &poll, nil
}

func (a *App) createPoll(c *gin.Context) {
	var in CreatePollInput
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Please send valid poll details.")
		return
	}
	if err := validatePoll(&in); err != nil {
		fail(c, 400, err.Error())
		return
	}
	user := c.MustGet("user").(*User)
	poll := Poll{ID: token()[:24], OwnerID: user.ID, Question: in.Question, Description: in.Description, Open: true, Version: 1, CreatedAt: time.Now().UTC(), Ballots: map[string]string{}, IsOwner: true, Capacity: maxVotes}
	for i, label := range in.Options {
		poll.Options = append(poll.Options, Option{ID: fmt.Sprintf("option-%d", i+1), Label: label})
	}
	ctx, cancel := dbContext(c)
	defer cancel()
	if _, err := a.db.Collection("polls").InsertOne(ctx, poll); err != nil {
		fail(c, 503, "Could not save your poll. Please retry.")
		return
	}
	c.JSON(201, poll)
}

func (a *App) listPolls(c *gin.Context) {
	ctx, cancel := dbContext(c)
	defer cancel()
	user := c.MustGet("user").(*User)
	cur, err := a.db.Collection("polls").Find(ctx, bson.M{"ownerId": user.ID}, options.Find().SetProjection(bson.M{"ballots": 0}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		fail(c, 503, "Could not load your polls.")
		return
	}
	defer cur.Close(ctx)
	polls := []Poll{}
	if err = cur.All(ctx, &polls); err != nil {
		fail(c, 503, "Could not load your polls.")
		return
	}
	for i := range polls {
		polls[i].IsOwner = true
		polls[i].Capacity = maxVotes
	}
	c.JSON(200, gin.H{"polls": polls})
}

func (a *App) getPoll(c *gin.Context) {
	if !validID(c.Param("id")) {
		fail(c, 404, "Poll not found.")
		return
	}
	visitor, owner, ok := a.identity(c)
	if !ok {
		return
	}
	ctx, cancel := dbContext(c)
	defer cancel()
	poll, err := a.readPoll(ctx, c.Param("id"), visitor, owner)
	if errors.Is(err, mongo.ErrNoDocuments) {
		fail(c, 404, "This poll does not exist. Check the link.")
		return
	}
	if err != nil {
		fail(c, 503, "Could not load this poll.")
		return
	}
	c.JSON(200, poll)
}

func (a *App) publish(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.redis.Publish(ctx, "pulsepoll:poll:"+id, "changed").Err(); err != nil {
		log.Printf("Live notification delayed for poll %s; SSE reconciliation will recover it", id)
	}
}

func (a *App) vote(c *gin.Context) {
	id := c.Param("id")
	if !validID(id) {
		fail(c, 404, "Poll not found.")
		return
	}
	var input struct {
		OptionID string `json:"optionId"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.OptionID) > 32 {
		fail(c, 400, "Choose a valid option.")
		return
	}
	// A prior GET establishes the browser identity, preventing parallel first
	// POST requests without cookies from each becoming a different guest.
	if _, err := c.Cookie("pulse_visitor"); err != nil {
		fail(c, 400, "Open the poll before voting.")
		return
	}
	visitor, owner, ok := a.identity(c)
	if !ok {
		return
	}
	ctx, cancel := dbContext(c)
	defer cancel()
	filter := bson.M{"_id": id, "open": true, "total": bson.M{"$lt": maxVotes}, "options.id": input.OptionID, "ballots." + visitor: bson.M{"$exists": false}}
	update := bson.M{"$inc": bson.M{"options.$.votes": 1, "total": 1, "version": 1}, "$set": bson.M{"ballots." + visitor: input.OptionID}}
	result, err := a.db.Collection("polls").UpdateOne(ctx, filter, update)
	if err != nil {
		fail(c, 503, "Could not confirm your vote. Reopen the poll before retrying.")
		return
	}
	poll, err := a.readPoll(ctx, id, visitor, owner)
	if errors.Is(err, mongo.ErrNoDocuments) {
		fail(c, 404, "Poll not found.")
		return
	}
	if err != nil {
		fail(c, 503, "Could not load the latest results. Reopen the poll to check your vote.")
		return
	}
	if result.ModifiedCount == 0 {
		// Repeating the same vote is idempotent: a network retry adds no vote.
		if poll.MyChoice == input.OptionID && poll.MyChoice != "" {
			c.JSON(200, poll)
			return
		}
		if poll.MyChoice != "" {
			fail(c, 409, "You have already voted from this browser.")
			return
		}
		if !poll.Open {
			fail(c, 409, "This poll is closed.")
			return
		}
		if poll.Total >= maxVotes {
			fail(c, 409, "This poll has reached its vote limit.")
			return
		}
		fail(c, 400, "This option does not belong to the poll.")
		return
	}
	a.publish(id)
	c.JSON(200, poll)
}

func (a *App) closePoll(c *gin.Context) {
	id := c.Param("id")
	if !validID(id) {
		fail(c, 404, "Poll not found.")
		return
	}
	user := c.MustGet("user").(*User)
	ctx, cancel := dbContext(c)
	defer cancel()
	result, err := a.db.Collection("polls").UpdateOne(ctx, bson.M{"_id": id, "ownerId": user.ID, "open": true}, bson.M{"$set": bson.M{"open": false}, "$inc": bson.M{"version": 1}})
	if err != nil {
		fail(c, 503, "Could not close this poll.")
		return
	}
	poll, err := a.readPoll(ctx, id, a.visitor(c), user.ID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		fail(c, 404, "Poll not found.")
		return
	}
	if err != nil {
		fail(c, 503, "Could not load the poll.")
		return
	}
	if !poll.IsOwner {
		fail(c, 403, "Only the creator can close this poll.")
		return
	}
	if result.ModifiedCount > 0 {
		a.publish(id)
	}
	c.JSON(200, poll)
}
