package model

import "time"

type Message struct {
	ID                int64
	SessionID         int64
	Channel           string
	ExternalMessageID string
	TelegramMessageID int64
	TelegramUpdateID  int64
	Role              string
	Content           string
	Mode              string
	CreatedAt         time.Time
}
