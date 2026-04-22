package model

import "time"

type User struct {
	ID             int64
	BotID          string
	Channel        string
	ExternalUserID string
	ExternalChatID string
	TelegramUserID int64
	TelegramChatID int64
	Username       string
	FirstName      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
