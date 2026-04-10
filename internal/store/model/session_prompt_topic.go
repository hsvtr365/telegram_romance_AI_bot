package model

import "time"

type SessionPromptTopic struct {
	ID        int64
	SessionID int64
	TopicKey  string
	TopicType string
	TopicDate time.Time
	Source    string
	UsedAt    time.Time
	CreatedAt time.Time
}
