package model

import "time"

type CustomSlot struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	SlotIndex int       `json:"slot_index"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
