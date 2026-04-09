package model

import "time"

type Session struct {
	ID              int64
	UserID          int64
	SessionStatus   string
	Mode            string
	RecentTurnLimit int
	LastMessageAt   time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
