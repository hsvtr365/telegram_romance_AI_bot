package proactive

import (
	"context"
	"strings"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type TriggerType string

const (
	TriggerReminder      TriggerType = "reminder"
	TriggerReconnect     TriggerType = "reconnect"
	TriggerEventFollowup TriggerType = "event_followup"
	TriggerMoodRepair    TriggerType = "mood_repair"
	TriggerHabitPing     TriggerType = "habit_ping"
)

type Intensity string

const (
	IntensityLight  Intensity = "light"
	IntensityMedium Intensity = "medium"
	IntensityBold   Intensity = "bold"
)

type ToneMode string

const (
	ToneSoft  ToneMode = "soft"
	ToneSpicy ToneMode = "spicy"
	ToneRough ToneMode = "rough"
)

type Purpose string

const (
	PurposeCheckin  Purpose = "checkin"
	PurposeFollowup Purpose = "followup"
	PurposeRepair   Purpose = "repair"
	PurposeTease    Purpose = "tease"
)

type Length string

const (
	LengthShort  Length = "short"
	LengthMedium Length = "medium"
)

type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliverySending   DeliveryStatus = "sending"
	DeliverySent      DeliveryStatus = "sent"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryDeferred  DeliveryStatus = "deferred"
	DeliverySupersede DeliveryStatus = "superseded"
)

type TriggerCandidate struct {
	CandidateID  string             `json:"candidate_id"`
	SessionID    int64              `json:"session_id"`
	UserID       int64              `json:"user_id"`
	TriggerType  TriggerType        `json:"trigger_type"`
	TriggerRefID string             `json:"trigger_ref_id"`
	Priority     int                `json:"priority"`
	TriggeredAt  time.Time          `json:"triggered_at"`
	DueAt        time.Time          `json:"due_at"`
	Source       string             `json:"source"`
	Metadata     map[string]string  `json:"metadata,omitempty"`
	ScoreHints   map[string]float64 `json:"score_hints,omitempty"`
}

type ScannedCandidate struct {
	Candidate        TriggerCandidate         `json:"candidate"`
	Session          SessionSnapshot          `json:"session"`
	Profile          ProactiveProfile         `json:"profile"`
	RecentMessages   []ConversationMessage    `json:"recent_messages,omitempty"`
	RecentProactives []ProactiveMessageRecord `json:"recent_proactives,omitempty"`
	Event            *MemoryEvent             `json:"event,omitempty"`
	Eligibility      *EligibilityResult       `json:"eligibility,omitempty"`
	CandidateScore   *ScoreResult             `json:"candidate_score,omitempty"`
}

type Strategy struct {
	Type      string    `json:"type"`
	Intensity Intensity `json:"intensity"`
	Tone      ToneMode  `json:"tone"`
	Purpose   Purpose   `json:"purpose"`
	Length    Length    `json:"length"`
}

type EligibilityResult struct {
	Eligible  bool     `json:"eligible"`
	HardBlock bool     `json:"hard_block"`
	Reasons   []string `json:"reasons,omitempty"`
}

type ScoreResult struct {
	Score   float64            `json:"score"`
	Base    float64            `json:"base"`
	Bonus   map[string]float64 `json:"bonus,omitempty"`
	Penalty map[string]float64 `json:"penalty,omitempty"`
	Reasons []string           `json:"reasons,omitempty"`
}

type DecisionAction string

const (
	DecisionSend      DecisionAction = "send"
	DecisionDefer     DecisionAction = "defer"
	DecisionDrop      DecisionAction = "drop"
	DecisionSkip      DecisionAction = "skip"
	DecisionSupersede DecisionAction = "supersede"
)

type Decision struct {
	Candidate TriggerCandidate  `json:"candidate"`
	Eligible  EligibilityResult `json:"eligible"`
	Score     ScoreResult       `json:"score"`
	Strategy  Strategy          `json:"strategy"`
	Action    DecisionAction    `json:"action"`
	Reason    string            `json:"reason"`
}

type QuietWindow struct {
	Weekdays []string `json:"weekdays,omitempty"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
}

type TimeWindow struct {
	Weekdays []string `json:"weekdays,omitempty"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Score    float64  `json:"score,omitempty"`
}

type SessionSnapshot struct {
	SessionID                   int64                   `json:"session_id"`
	UserID                      int64                   `json:"user_id"`
	Target                      channelx.OutboundTarget `json:"target"`
	TelegramChatID              int64                   `json:"telegram_chat_id"`
	Mode                        string                  `json:"mode"`
	LastUserMessageAt           time.Time               `json:"last_user_message_at"`
	LastBotMessageAt            time.Time               `json:"last_bot_message_at"`
	LastProactiveAt             time.Time               `json:"last_proactive_at"`
	LastUserReplyToProactiveAt  time.Time               `json:"last_user_reply_to_proactive_at"`
	ConsecutiveProactiveIgnored int                     `json:"consecutive_proactive_ignored"`
	RelationshipScore           float64                 `json:"relationship_score"`
	CurrentMood                 string                  `json:"current_mood"`
	ProactiveOptIn              bool                    `json:"proactive_opt_in"`
	QuietHours                  []QuietWindow           `json:"quiet_hours,omitempty"`
	TimezoneName                string                  `json:"timezone_name"`
	LastMessageAt               time.Time               `json:"last_message_at"`
	RecentTurnLimit             int                     `json:"recent_turn_limit"`
	UpdatedAt                   time.Time               `json:"updated_at"`
}

type ConversationMessage struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
}

type MemoryEvent struct {
	ID               int64          `json:"id"`
	SessionID        int64          `json:"session_id"`
	MessageID        int64          `json:"message_id"`
	EventType        string         `json:"event_type"`
	EventSubtype     string         `json:"event_subtype"`
	EventValue       map[string]any `json:"event_value,omitempty"`
	EventTime        time.Time      `json:"event_time"`
	Priority         int            `json:"priority"`
	UsedForProactive bool           `json:"used_for_proactive"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (m MemoryEvent) PreparedMessage() string {
	if len(m.EventValue) == 0 {
		return ""
	}
	value, ok := m.EventValue["prepared_message"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

type ProactiveProfile struct {
	UserID                int64              `json:"user_id"`
	PreferredTypes        []string           `json:"preferred_types,omitempty"`
	DislikedTypes         []string           `json:"disliked_types,omitempty"`
	BestTimeWindows       []TimeWindow       `json:"best_time_windows,omitempty"`
	MaxPerDay             int                `json:"max_per_day"`
	MaxPerWeek            int                `json:"max_per_week"`
	MinGapHours           int                `json:"min_gap_hours"`
	AvgReplyDelaySec      int                `json:"avg_reply_delay_sec"`
	ProactiveSuccessScore float64            `json:"proactive_success_score"`
	TimezoneName          string             `json:"timezone_name"`
	WeightOverrides       map[string]float64 `json:"weight_overrides,omitempty"`
}

type Seed struct {
	Key  string
	Text string
	Meta map[string]string
}

type ProactiveMessageRecord struct {
	ID                int64          `json:"id"`
	SessionID         int64          `json:"session_id"`
	UserID            int64          `json:"user_id"`
	TriggerType       TriggerType    `json:"trigger_type"`
	TriggerRefID      string         `json:"trigger_ref_id"`
	StrategyType      string         `json:"strategy_type"`
	ToneMode          ToneMode       `json:"tone_mode"`
	Intensity         Intensity      `json:"intensity"`
	Purpose           Purpose        `json:"purpose"`
	Score             float64        `json:"score"`
	MessageText       string         `json:"message_text"`
	SeedKey           string         `json:"seed_key,omitempty"`
	Channel           string         `json:"channel,omitempty"`
	ExternalMessageID string         `json:"external_message_id,omitempty"`
	TelegramMessageID int64          `json:"telegram_message_id,omitempty"`
	SentAt            time.Time      `json:"sent_at,omitempty"`
	DeliveryStatus    DeliveryStatus `json:"delivery_status"`
	UserReplied       bool           `json:"user_replied"`
	ReplyDelaySec     int            `json:"reply_delay_sec,omitempty"`
	ReplySentiment    string         `json:"reply_sentiment,omitempty"`
	ReplyLength       int            `json:"reply_length,omitempty"`
	FollowupTurnCount int            `json:"followup_turn_count,omitempty"`
	CreatedAt         time.Time      `json:"created_at,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at,omitempty"`
}

type FeedbackResult struct {
	Matched               bool
	ReplyDelaySec         int
	ReplyLength           int
	ReplySentiment        string
	FollowupTurnCount     int
	RelationshipDelta     float64
	TypeBiasDelta         float64
	TimeWindowBiasDelta   float64
	ShouldMarkUserReplied bool
	Reasons               []string
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Repository interface {
	ListSessionsForProactive(ctx context.Context, now time.Time) ([]SessionSnapshot, error)
	ListDueMemoryEvents(ctx context.Context, now time.Time, limit int) ([]MemoryEvent, error)
	ListRecentMessages(ctx context.Context, sessionID int64, limit int) ([]ConversationMessage, error)
	ListRecentProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]ProactiveMessageRecord, error)
	GetMemorySummary(ctx context.Context, sessionID int64, recentTurnLimit int) (string, error)
	ListSessionCustomSlots(ctx context.Context, sessionID int64) ([]model.CustomSlot, error)
	GetProactiveProfile(ctx context.Context, userID int64) (ProactiveProfile, error)
	InsertProactiveMessage(ctx context.Context, record ProactiveMessageRecord) (int64, error)
	UpdateProactiveMessage(ctx context.Context, record ProactiveMessageRecord) error
	SaveConversationTurn(ctx context.Context, sessionID int64, role string, content string, mode string, recentTurnLimit int) error
	MarkMemoryEventUsedForProactive(ctx context.Context, eventID int64) error
	UpdateSessionProactiveState(ctx context.Context, sessionID int64, patch SessionProactivePatch) error
	UpdateProactiveProfile(ctx context.Context, userID int64, patch ProactiveProfilePatch) error
}

type SessionProactivePatch struct {
	LastUserMessageAt           *time.Time
	LastBotMessageAt            *time.Time
	LastProactiveAt             *time.Time
	LastUserReplyToProactiveAt  *time.Time
	ConsecutiveProactiveIgnored *int
	RelationshipScore           *float64
	CurrentMood                 *string
	ProactiveOptIn              *bool
	QuietHours                  []QuietWindow
}

type ProactiveProfilePatch struct {
	PreferredTypes        []string
	DislikedTypes         []string
	BestTimeWindows       []TimeWindow
	MaxPerDay             *int
	MaxPerWeek            *int
	MinGapHours           *int
	AvgReplyDelaySec      *int
	ProactiveSuccessScore *float64
	TimezoneName          *string
	WeightOverrides       map[string]float64
}

type Messenger = channelx.Messenger

type AudioGenerator interface {
	GenerateAudio(ctx context.Context, transcript string) (channelx.AudioAttachment, error)
}

type LLM interface {
	Chat(ctx context.Context, messages []ollama.Message) (string, error)
}
