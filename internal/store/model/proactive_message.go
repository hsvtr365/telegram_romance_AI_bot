package model

import "time"

type ProactiveMessage struct {
	ID                int64
	SessionID         int64
	UserID            int64
	TriggerType       string
	TriggerRefID      string
	StrategyType      string
	ToneMode          string
	Intensity         string
	Purpose           string
	Score             float64
	MessageText       string
	SeedKey           string
	TelegramMessageID int64
	SentAt            time.Time
	DeliveryStatus    string
	UserReplied       bool
	ReplyDelaySec     int
	ReplySentiment    string
	ReplyLength       int
	FollowupTurnCount int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
