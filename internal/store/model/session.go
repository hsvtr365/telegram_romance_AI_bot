package model

import "time"

type Session struct {
	ID                          int64
	UserID                      int64
	SessionStatus               string
	Mode                        string
	RecentTurnLimit             int
	LastMessageAt               time.Time
	LastUserMessageAt           time.Time
	LastBotMessageAt            time.Time
	LastProactiveAt             time.Time
	LastUserReplyToProactiveAt  time.Time
	ConsecutiveProactiveIgnored int
	RelationshipScore           float64
	CurrentMood                 string
	ProactiveOptIn              bool
	QuietHoursJSON              []byte
	HistorySummary              string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}
