package model

import (
	"encoding/json"
	"time"
)

type MemoryEvent struct {
	ID               int64           `json:"id"`
	SessionID        int64           `json:"session_id"`
	MessageID        int64           `json:"message_id"`
	EventType        string          `json:"event_type"`
	EventSubtype     string          `json:"event_subtype"`
	EventValue       json.RawMessage `json:"event_value"`
	EventTime        time.Time       `json:"event_time"`
	Priority         int             `json:"priority"`
	UsedForProactive bool            `json:"used_for_proactive"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
