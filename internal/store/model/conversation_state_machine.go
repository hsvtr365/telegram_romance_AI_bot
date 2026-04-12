package model

import (
	"encoding/json"
	"time"
)

type ConversationStateMachine struct {
	SessionID           int64
	Revision            int64
	TonePhase           string
	RelationalStage     string
	StageDirection      string
	EmotionalTone       string
	InteractionMode     string
	FocusTopicKey       string
	OpenLoopSummary     string
	SafetyLockUntilTurn int
	Confidence          string
	LastSourceMessageID int64
	LastDecisionSource  string
	EvidenceJSON        json.RawMessage
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
