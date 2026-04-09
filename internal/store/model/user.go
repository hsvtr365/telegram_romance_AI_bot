package model

import "time"

type User struct {
	ID             int64
	TelegramUserID int64
	TelegramChatID int64
	Username       string
	FirstName      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
