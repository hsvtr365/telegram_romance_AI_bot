package model

import (
	"encoding/json"
	"time"
)

const (
	TopicSlotStatusActive   = "active"
	TopicSlotStatusWatch    = "watch"
	TopicSlotStatusResolved = "resolved"
	TopicSlotStatusArchived = "archived"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

type TopicSlot struct {
	ID                  int64
	SessionID           int64
	SlotKey             string
	TopicLabel          string
	Summary             string
	Status              string
	Importance          int
	Confidence          string
	SourceKind          string
	FirstSeenAt         time.Time
	LastSeenAt          time.Time
	LastSourceMessageID int64
	MentionCount        int
	EvidenceJSON        json.RawMessage
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AnalyzedTopicSlot struct {
	Label         string   `json:"label"`
	Summary       string   `json:"summary"`
	Status        string   `json:"status"`
	Importance    int      `json:"importance"`
	Confidence    string   `json:"confidence"`
	EvidenceTexts []string `json:"evidence_texts"`
	Supersedes    []string `json:"supersedes"`
}
