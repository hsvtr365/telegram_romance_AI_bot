package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App       AppConfig
	Telegram  TelegramConfig
	Ollama    OllamaConfig
	Storage   StorageConfig
	Chat      ChatConfig
	Proactive ProactiveConfig
	Holiday   HolidayConfig
}

type AppConfig struct {
	Name string
	Env  string
	Port int
}

type TelegramConfig struct {
	BotToken       string
	AllowedUpdates []string
	PollTimeoutSec int
	PollLimit      int
}

type OllamaConfig struct {
	BaseURL     string
	Model       string
	TimeoutSec  int
	KeepAlive   string
	NumCtx      int
	Temperature float64
	TopP        float64
}

type StorageConfig struct {
	PostgresDSN string
	RedisURL    string
}

type ChatConfig struct {
	RecentTurnLimit          int
	SummaryTriggerMessages   int
	SessionLockTTLSec        int
	CooldownSec              int
	ResponseMaxChars         int
	PhaseRulesPath           string
	StateReviewEnabled       bool
	StateReviewModel         string
	StateReviewBaseURL       string
	StateReviewTimeoutMs     int
	StateReviewWorkers       int
	StateReviewQueueSize     int
	StructuredExtract        bool
	StructuredMinChars       int
	StructuredModel          string
	StructuredBaseURL        string
	MemorySlotEnabled        bool
	MemorySlotModel          string
	MemorySlotBaseURL        string
	MemorySlotMinChars       int
	MemorySlotSyncTimeoutMs  int
	MemorySlotAsyncTimeoutMs int
	MemorySlotWorkers        int
	MemorySlotQueueSize      int
}

type ProactiveConfig struct {
	Enabled                 bool
	ScanIntervalSec         int
	ReminderScanIntervalSec int
	FeedbackIntervalSec     int
	ReplyWindowHours        int
	DefaultTimezone         string
	AllowedStartHour        int
	AllowedEndHour          int
	ReconnectIdleHours      int
	EventLookaheadHours     int
	EventFollowupGraceHours int
	MaxCandidatesPerScan    int
	ReminderModel           string
	ReminderBaseURL         string
}

type HolidayConfig struct {
	SyncEnabled       bool
	APIServiceKey     string
	APIBaseURL        string
	SyncIntervalHours int
	LookaheadDays     int
	PromptTodayPct    int
	PromptUpcomingPct int
}

func Load(dotenvPath string) (Config, error) {
	if err := loadDotEnv(dotenvPath); err != nil {
		return Config{}, err
	}

	cfg := Config{
		App: AppConfig{
			Name: envString("APP_NAME", "heartlink-bot"),
			Env:  envString("APP_ENV", "local"),
			Port: envInt("APP_PORT", 8080),
		},
		Telegram: TelegramConfig{
			BotToken:       strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			AllowedUpdates: envCSV("TELEGRAM_ALLOWED_UPDATES", []string{"message"}),
			PollTimeoutSec: envInt("TELEGRAM_POLL_TIMEOUT_SEC", 30),
			PollLimit:      envInt("TELEGRAM_POLL_LIMIT", 50),
		},
		Ollama: OllamaConfig{
			BaseURL:     envString("OLLAMA_BASE_URL", "http://127.0.0.1:11434"),
			Model:       envString("OLLAMA_MODEL", "gemma4:4b"),
			TimeoutSec:  envInt("OLLAMA_TIMEOUT_SEC", 35),
			KeepAlive:   envString("OLLAMA_KEEP_ALIVE", "10m"),
			NumCtx:      envInt("OLLAMA_NUM_CTX", 4096),
			Temperature: envFloat("OLLAMA_TEMPERATURE", 0.9),
			TopP:        envFloat("OLLAMA_TOP_P", 0.9),
		},
		Storage: StorageConfig{
			PostgresDSN: strings.TrimSpace(os.Getenv("POSTGRES_DSN")),
			RedisURL:    strings.TrimSpace(os.Getenv("REDIS_URL")),
		},
		Chat: ChatConfig{
			RecentTurnLimit:          envInt("RECENT_TURN_LIMIT", 14),
			SummaryTriggerMessages:   envInt("SUMMARY_TRIGGER_MESSAGES", 10),
			SessionLockTTLSec:        envInt("SESSION_LOCK_TTL_SEC", 20),
			CooldownSec:              envInt("CHAT_COOLDOWN_SEC", 2),
			ResponseMaxChars:         envInt("RESPONSE_MAX_CHARS", 0),
			PhaseRulesPath:           envString("CHAT_PHASE_RULES_PATH", "configs/conversation_phase_rules.json"),
			StateReviewEnabled:       envBool("CHAT_STATE_REVIEW_ENABLED", true),
			StateReviewModel:         envString("CHAT_STATE_REVIEW_MODEL", ""),
			StateReviewBaseURL:       envString("CHAT_STATE_REVIEW_BASE_URL", ""),
			StateReviewTimeoutMs:     envInt("CHAT_STATE_REVIEW_TIMEOUT_MS", 1200),
			StateReviewWorkers:       envInt("CHAT_STATE_REVIEW_WORKERS", 2),
			StateReviewQueueSize:     envInt("CHAT_STATE_REVIEW_QUEUE_SIZE", 32),
			StructuredExtract:        envBool("CHAT_STRUCTURED_EXTRACT_ENABLED", true),
			StructuredMinChars:       envInt("CHAT_STRUCTURED_EXTRACT_MIN_CHARS", 1),
			StructuredModel:          envString("CHAT_STRUCTURED_EXTRACT_MODEL", ""),
			StructuredBaseURL:        envString("CHAT_STRUCTURED_EXTRACT_BASE_URL", "http://127.0.0.1:11434"),
			MemorySlotEnabled:        envBool("CHAT_MEMORY_SLOT_ENABLED", true),
			MemorySlotModel:          envString("CHAT_MEMORY_SLOT_MODEL", ""),
			MemorySlotBaseURL:        envString("CHAT_MEMORY_SLOT_BASE_URL", "http://127.0.0.1:11434"),
			MemorySlotMinChars:       envInt("CHAT_MEMORY_SLOT_MIN_CHARS", 5),
			MemorySlotSyncTimeoutMs:  envInt("CHAT_MEMORY_SLOT_SYNC_TIMEOUT_MS", 0),
			MemorySlotAsyncTimeoutMs: envInt("CHAT_MEMORY_SLOT_ASYNC_TIMEOUT_MS", 120000),
			MemorySlotWorkers:        envInt("CHAT_MEMORY_SLOT_WORKERS", 2),
			MemorySlotQueueSize:      envInt("CHAT_MEMORY_SLOT_QUEUE_SIZE", 32),
		},
		Proactive: ProactiveConfig{
			Enabled:                 envBool("PROACTIVE_ENABLED", true),
			ScanIntervalSec:         envInt("PROACTIVE_SCAN_INTERVAL_SEC", 60),
			ReminderScanIntervalSec: envInt("PROACTIVE_REMINDER_SCAN_INTERVAL_SEC", 5),
			FeedbackIntervalSec:     envInt("PROACTIVE_FEEDBACK_INTERVAL_SEC", 300),
			ReplyWindowHours:        envInt("PROACTIVE_REPLY_WINDOW_HOURS", 24),
			DefaultTimezone:         envString("PROACTIVE_DEFAULT_TIMEZONE", "Asia/Seoul"),
			AllowedStartHour:        envInt("PROACTIVE_ALLOWED_START_HOUR", 11),
			AllowedEndHour:          envInt("PROACTIVE_ALLOWED_END_HOUR", 22),
			ReconnectIdleHours:      envInt("PROACTIVE_RECONNECT_IDLE_HOURS", 72),
			EventLookaheadHours:     envInt("PROACTIVE_EVENT_LOOKAHEAD_HOURS", 3),
			EventFollowupGraceHours: envInt("PROACTIVE_EVENT_FOLLOWUP_GRACE_HOURS", 3),
			MaxCandidatesPerScan:    envInt("PROACTIVE_MAX_CANDIDATES_PER_SCAN", 100),
			ReminderModel:           envString("PROACTIVE_REMINDER_MODEL", ""),
			ReminderBaseURL:         envString("PROACTIVE_REMINDER_BASE_URL", "http://127.0.0.1:11434"),
		},
		Holiday: HolidayConfig{
			SyncEnabled:       envBool("HOLIDAY_SYNC_ENABLED", true),
			APIServiceKey:     strings.TrimSpace(os.Getenv("HOLIDAY_API_SERVICE_KEY")),
			APIBaseURL:        envString("HOLIDAY_API_BASE_URL", "http://apis.data.go.kr/B090041/openapi/service/SpcdeInfoService"),
			SyncIntervalHours: envInt("HOLIDAY_SYNC_INTERVAL_HOURS", 24),
			LookaheadDays:     envInt("HOLIDAY_LOOKAHEAD_DAYS", 7),
			PromptTodayPct:    envInt("HOLIDAY_PROMPT_TODAY_PERCENT", 40),
			PromptUpcomingPct: envInt("HOLIDAY_PROMPT_UPCOMING_PERCENT", 25),
		},
	}

	return cfg, nil
}

func (c Config) Validate() error {
	var missing []string

	if c.Telegram.BotToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}

	if c.Ollama.BaseURL == "" {
		missing = append(missing, "OLLAMA_BASE_URL")
	}

	if c.Ollama.Model == "" {
		missing = append(missing, "OLLAMA_MODEL")
	}

	if c.Storage.PostgresDSN == "" {
		missing = append(missing, "POSTGRES_DSN")
	}

	if c.Storage.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}

	if c.Holiday.SyncEnabled && c.Holiday.APIServiceKey == "" {
		missing = append(missing, "HOLIDAY_API_SERVICE_KEY")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required env values: %s", strings.Join(missing, ", "))
	}

	return nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envCSV(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}

	if len(result) == 0 {
		return fallback
	}

	return result
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
