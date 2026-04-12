package model

import "time"

const (
	ProfileCandidateStatusActive    = "active"
	ProfileCandidateStatusConfirmed = "confirmed"
	ProfileCandidateStatusDismissed = "dismissed"
)

type ProfileCandidateEvidence struct {
	MessageID    int64     `json:"message_id"`
	EvidenceText string    `json:"evidence_text"`
	EvidenceType string    `json:"evidence_type"`
	Confidence   string    `json:"confidence"`
	CapturedAt   time.Time `json:"captured_at"`
}

type ProfileCandidate struct {
	ID               int64
	UserID           int64
	SlotName         string
	CandidateValue   string
	NormalizedValue  string
	BestEvidenceType string
	BestConfidence   string
	EvidenceMessages []ProfileCandidateEvidence
	MentionCount     int
	FirstSeenAt      time.Time
	LastSeenAt       time.Time
	Status           string
	ReviewedAt       time.Time
	PromotedAt       time.Time
}
