package model

import (
	"encoding/json"
	"time"
)

type ProactiveProfile struct {
	ID                    int64
	UserID                int64
	PreferredTypesJSON    json.RawMessage
	DislikedTypesJSON     json.RawMessage
	BestTimeWindowsJSON   json.RawMessage
	MaxPerDay             int
	MaxPerWeek            int
	MinGapHours           int
	AvgReplyDelaySec      int
	ProactiveSuccessScore float64
	TimezoneName          string
	WeightOverridesJSON   json.RawMessage
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
