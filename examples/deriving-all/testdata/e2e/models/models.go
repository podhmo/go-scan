package models

import "time"

// @deriving:unmarshal
// @deriving:binding in:"body"
type User struct {
	// User's unique identifier
	ID string `json:"id"`
	// User's full name
	Name string `json:"name"`
	// User's birth date
	BirthDate time.Time `json:"birthDate"`
}

type EventData interface {
	isEventData()
}

// @deriving:unmarshal
type Event struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Data      EventData `json:"data"`
}

// @deriving:marshal
type UserCreated struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

func (UserCreated) isEventData() {}

// @deriving:marshal
type MessagePosted struct {
	MessageID string `json:"messageId"`
	Content   string `json:"content"`
}

func (MessagePosted) isEventData() {}
