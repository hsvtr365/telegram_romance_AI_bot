package model

import (
	"encoding/json"
	"time"
)

type ConversationStateSlot struct {
	SessionID           int64
	CurrentStage        string
	StageDirection      string
	EmotionalTone       string
	InteractionMode     string
	OpenLoopSummary     string
	FocusTopicKey       string
	Confidence          string
	EvidenceJSON        json.RawMessage
	LastSourceMessageID int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AnalyzedConversationState struct {
	CurrentStage    string   `json:"current_stage"`
	StageDirection  string   `json:"stage_direction"`
	EmotionalTone   string   `json:"emotional_tone"`
	InteractionMode string   `json:"interaction_mode"`
	OpenLoopSummary string   `json:"open_loop_summary"`
	FocusTopicLabel string   `json:"focus_topic_label"`
	Confidence      string   `json:"confidence"`
	EvidenceTexts   []string `json:"evidence_texts"`
}
