package main

import "time"

// The ballot map and counters live in one MongoDB document. A conditional
// update records a ballot and increments its counters atomically.
const maxVotes int64 = 10000

type User struct {
	ID           string `bson:"_id" json:"id"`
	Name         string `bson:"name" json:"name"`
	Email        string `bson:"email" json:"email"`
	PasswordHash string `bson:"passwordHash" json:"-"`
}

type Session struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

type Option struct {
	ID    string `bson:"id" json:"id"`
	Label string `bson:"label" json:"label"`
	Votes int64  `bson:"votes" json:"votes"`
}

type Poll struct {
	ID          string            `bson:"_id" json:"id"`
	OwnerID     string            `bson:"ownerId" json:"-"`
	Question    string            `bson:"question" json:"question"`
	Description string            `bson:"description" json:"description"`
	Options     []Option          `bson:"options" json:"options"`
	Open        bool              `bson:"open" json:"open"`
	Total       int64             `bson:"total" json:"total"`
	Version     int64             `bson:"version" json:"version"`
	CreatedAt   time.Time         `bson:"createdAt" json:"createdAt"`
	Ballots     map[string]string `bson:"ballots,omitempty" json:"-"`
	MyChoice    string            `bson:"-" json:"myChoice"`
	IsOwner     bool              `bson:"-" json:"isOwner"`
	Capacity    int64             `bson:"-" json:"capacity"`
}

type CreatePollInput struct {
	Question    string   `json:"question"`
	Description string   `json:"description"`
	Options     []string `json:"options"`
}
