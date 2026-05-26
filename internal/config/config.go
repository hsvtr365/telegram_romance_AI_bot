package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	App       AppConfig
	Telegram  TelegramConfig
	Bots      []BotConfig
	Ollama    OllamaConfig
	Gemini    GeminiConfig
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
	AllowedUpdates []string
	PollTimeoutSec int
	PollLimit      int
}

type BotConfig struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	TelegramBotToken   string          `json:"telegram_bot_token"`
	PersonaPromptPath  string          `json:"persona_prompt_path"`
	PersonaPrompt      string          `json:"-"`
	PolicyPath         string          `json:"policy_path"`
	Channels           []ChannelConfig `json:"channels"`
	RewardTTSEnabled   bool            `json:"reward_tts_enabled"`
	DefaultChatMode    string          `json:"default_chat_mode"`
	WelcomeText        string          `json:"welcome_text"`
	FallbackText       string          `json:"fallback_text"`
	ResetText          string          `json:"reset_text"`
	ResetOpeningText   string          `json:"reset_opening_text"`
	ProactiveOnText    string          `json:"proactive_on_text"`
	ProactiveOffText   string          `json:"proactive_off_text"`
	ProactiveErrorText string          `json:"proactive_error_text"`
}

type ChannelConfig struct {
	Type  string `json:"type"`
	Token string `json:"token"`
	Mode  string `json:"mode"`
}

type botsConfigFile struct {
	CommonPromptPath string      `json:"common_prompt_path"`
	Bots             []BotConfig `json:"bots"`
}

type OllamaConfig struct {
	Endpoints              []OllamaEndpoint
	BaseURL                string
	BaseURLs               []string
	FallbackBaseURL        string
	HealthCheckIntervalSec int
	Model                  string
	TimeoutSec             int
	KeepAlive              string
	NumCtx                 int
	Temperature            float64
	TopP                   float64
}

type OllamaEndpoint struct {
	BaseURL string
	Model   string
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
	RewardTTSMinFollowups   int
}

type GeminiConfig struct {
	APIKey          string
	TTSModel        string
	TTSVoiceName    string
	TTSAudioProfile string
	TTSTimeoutSec   int
	TTSDebugDir     string
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

	baseURL := envString("OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	baseURLs := envCSV("OLLAMA_BASE_URLS", nil)
	fallbackBaseURL := envString("OLLAMA_FALLBACK_BASE_URL", "")
	model := envString("OLLAMA_MODEL", "gemma4:4b")
	endpoints := envOllamaEndpoints()
	if len(baseURLs) == 0 {
		baseURLs = uniqueNonEmptyStrings(baseURL, fallbackBaseURL)
	}
	if len(baseURLs) == 0 {
		baseURLs = []string{"http://127.0.0.1:11434"}
	}
	if len(endpoints) == 0 {
		for _, currentBaseURL := range baseURLs {
			endpoints = append(endpoints, OllamaEndpoint{
				BaseURL: currentBaseURL,
				Model:   model,
			})
		}
	}
	if len(endpoints) > 0 {
		baseURLs = endpointBaseURLs(endpoints)
		baseURL = endpoints[0].BaseURL
		model = endpoints[0].Model
	}

	cfg := Config{
		App: AppConfig{
			Name: envString("APP_NAME", "heartlink-bot"),
			Env:  envString("APP_ENV", "local"),
			Port: envInt("APP_PORT", 8080),
		},
		Telegram: TelegramConfig{
			AllowedUpdates: envCSV("TELEGRAM_ALLOWED_UPDATES", []string{"message"}),
			PollTimeoutSec: envInt("TELEGRAM_POLL_TIMEOUT_SEC", 30),
			PollLimit:      envInt("TELEGRAM_POLL_LIMIT", 50),
		},
		Ollama: OllamaConfig{
			Endpoints:              endpoints,
			BaseURL:                baseURL,
			BaseURLs:               baseURLs,
			FallbackBaseURL:        fallbackBaseURL,
			HealthCheckIntervalSec: envInt("OLLAMA_HEALTHCHECK_INTERVAL_SEC", 1800),
			Model:                  model,
			TimeoutSec:             envInt("OLLAMA_TIMEOUT_SEC", 35),
			KeepAlive:              envString("OLLAMA_KEEP_ALIVE", "10m"),
			NumCtx:                 envInt("OLLAMA_NUM_CTX", 4096),
			Temperature:            envFloat("OLLAMA_TEMPERATURE", 0.9),
			TopP:                   envFloat("OLLAMA_TOP_P", 0.9),
		},
		Gemini: GeminiConfig{
			APIKey:          strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
			TTSModel:        envString("GEMINI_TTS_MODEL", "gemini-3.1-flash-tts-preview"),
			TTSVoiceName:    envString("GEMINI_TTS_VOICE_NAME", "Charon"),
			TTSAudioProfile: envString("GEMINI_TTS_AUDIO_PROFILE", ""),
			TTSTimeoutSec:   envInt("GEMINI_TTS_TIMEOUT_SEC", 45),
			TTSDebugDir:     envString("GEMINI_TTS_DEBUG_DIR", ""),
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
			StateReviewTimeoutMs:     envInt("CHAT_STATE_REVIEW_TIMEOUT_MS", 120000), // Default 120 seconds, local LLMs are slow
			StateReviewWorkers:       envInt("CHAT_STATE_REVIEW_WORKERS", 1),
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
			RewardTTSMinFollowups:   envInt("PROACTIVE_REWARD_TTS_MIN_FOLLOWUPS", 1),
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

	bots, err := loadBotConfigs(dotenvPath)
	if err != nil {
		return Config{}, err
	}
	cfg.Bots = bots

	return cfg, nil
}

func (c Config) Validate() error {
	var missing []string

	if len(c.Bots) == 0 {
		missing = append(missing, "TELEGRAM_BOTS_CONFIG_PATH or TELEGRAM_BOT_TOKEN")
	}

	seenBotIDs := make(map[string]struct{}, len(c.Bots))
	for _, bot := range c.Bots {
		if bot.ID == "" {
			missing = append(missing, "bots[].id")
		} else if _, ok := seenBotIDs[bot.ID]; ok {
			missing = append(missing, "duplicate bot id: "+bot.ID)
		} else {
			seenBotIDs[bot.ID] = struct{}{}
		}
		if len(bot.Channels) == 0 {
			missing = append(missing, "bots["+bot.ID+"].channels")
		}
		for idx, channel := range bot.Channels {
			if channel.Type == "" {
				missing = append(missing, fmt.Sprintf("bots[%s].channels[%d].type", bot.ID, idx))
			}
			switch channel.Type {
			case "telegram":
				if channel.Token == "" {
					missing = append(missing, fmt.Sprintf("bots[%s].channels[%d].token", bot.ID, idx))
				}
			case "discord":
				if channel.Token == "" {
					missing = append(missing, fmt.Sprintf("bots[%s].channels[%d].token", bot.ID, idx))
				}
			default:
				missing = append(missing, fmt.Sprintf("bots[%s].channels[%d].type unsupported: %s", bot.ID, idx, channel.Type))
			}
		}
		if bot.PersonaPromptPath == "" {
			missing = append(missing, "bots["+bot.ID+"].persona_prompt_path")
		}
		if bot.PersonaPrompt == "" {
			missing = append(missing, "bots["+bot.ID+"].persona_prompt")
		}
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

func loadBotConfigs(dotenvPath string) ([]BotConfig, error) {
	path := envString("TELEGRAM_BOTS_CONFIG_PATH", "")
	if path == "" {
		defaultPath := filepath.Join(filepath.Dir(dotenvPath), "secrets/bots.local.json")
		if _, err := os.Stat(defaultPath); err == nil {
			path = defaultPath
		}
	}
	if path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(dotenvPath), path)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read telegram bots config: %w", err)
		}

		var file botsConfigFile
		if err := json.Unmarshal(payload, &file); err != nil {
			return nil, fmt.Errorf("parse telegram bots config: %w", err)
		}

		personaBaseDir := filepath.Dir(dotenvPath)
		commonPrompt, err := readPromptFile(personaBaseDir, strings.TrimSpace(file.CommonPromptPath))
		if err != nil {
			return nil, err
		}
		for idx := range file.Bots {
			normalizeBotConfig(&file.Bots[idx])
			prompt, err := readPromptFile(personaBaseDir, file.Bots[idx].PersonaPromptPath)
			if err != nil {
				return nil, err
			}
			file.Bots[idx].PersonaPrompt = combinePrompts(commonPrompt, prompt)
		}
		return file.Bots, nil
	}

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return nil, nil
	}

	bot := BotConfig{
		ID:                envString("TELEGRAM_BOT_ID", "seo-taegyu"),
		Name:              envString("TELEGRAM_BOT_NAME", "서태규"),
		TelegramBotToken:  token,
		PersonaPromptPath: envString("TELEGRAM_PERSONA_PROMPT_PATH", "configs/personas/seo-taegyu.md"),
		DefaultChatMode:   envString("DEFAULT_CHAT_MODE", "spicy"),
		WelcomeText:       "이제 왔어? 늦었네. 그래도 왔으니까 봐줄게.",
		FallbackText:      "잠깐만, 지금 답 고르는 중이야. 한 번만 더 툭 던져봐.",
		ResetText:         "리셋했어. 아까까지 했던 말은 다 지웠고, 지금부터 처음 본 것처럼 다시 시작할게.",
	}
	normalizeBotConfig(&bot)

	commonPrompt, err := readPromptFile(filepath.Dir(dotenvPath), envString("TELEGRAM_COMMON_PROMPT_PATH", ""))
	if err != nil {
		return nil, err
	}
	prompt, err := readPromptFile(filepath.Dir(dotenvPath), bot.PersonaPromptPath)
	if err != nil {
		return nil, err
	}
	bot.PersonaPrompt = combinePrompts(commonPrompt, prompt)
	return []BotConfig{bot}, nil
}

func normalizeBotConfig(bot *BotConfig) {
	bot.ID = strings.TrimSpace(bot.ID)
	bot.Name = strings.TrimSpace(bot.Name)
	bot.TelegramBotToken = strings.TrimSpace(bot.TelegramBotToken)
	bot.TelegramBotToken = expandEnvReference(bot.TelegramBotToken)
	bot.PersonaPromptPath = strings.TrimSpace(bot.PersonaPromptPath)
	bot.PersonaPrompt = strings.TrimSpace(bot.PersonaPrompt)
	bot.PolicyPath = strings.TrimSpace(bot.PolicyPath)
	bot.DefaultChatMode = strings.TrimSpace(bot.DefaultChatMode)
	bot.WelcomeText = strings.TrimSpace(bot.WelcomeText)
	bot.FallbackText = strings.TrimSpace(bot.FallbackText)
	bot.ResetText = strings.TrimSpace(bot.ResetText)
	bot.ResetOpeningText = strings.TrimSpace(bot.ResetOpeningText)
	bot.ProactiveOnText = strings.TrimSpace(bot.ProactiveOnText)
	bot.ProactiveOffText = strings.TrimSpace(bot.ProactiveOffText)
	bot.ProactiveErrorText = strings.TrimSpace(bot.ProactiveErrorText)
	for idx := range bot.Channels {
		bot.Channels[idx].Type = strings.TrimSpace(strings.ToLower(bot.Channels[idx].Type))
		bot.Channels[idx].Token = expandEnvReference(strings.TrimSpace(bot.Channels[idx].Token))
		bot.Channels[idx].Mode = strings.TrimSpace(strings.ToLower(bot.Channels[idx].Mode))
	}

	if bot.ID == "" {
		bot.ID = "default"
	}
	if bot.Name == "" {
		bot.Name = bot.ID
	}
	if bot.DefaultChatMode == "" {
		bot.DefaultChatMode = "spicy"
	}
	if len(bot.Channels) == 0 && bot.TelegramBotToken != "" {
		bot.Channels = []ChannelConfig{{
			Type:  "telegram",
			Token: bot.TelegramBotToken,
		}}
	}
	if bot.TelegramBotToken == "" {
		for _, channel := range bot.Channels {
			if channel.Type == "telegram" {
				bot.TelegramBotToken = channel.Token
				break
			}
		}
	}
	if bot.WelcomeText == "" {
		bot.WelcomeText = "안녕. 왔구나."
	}
	if bot.FallbackText == "" {
		bot.FallbackText = "잠깐만, 다시 한 번 말해줘."
	}
	if bot.ResetText == "" {
		bot.ResetText = "리셋했어. 지금부터 다시 시작할게."
	}
	if bot.ResetOpeningText == "" {
		bot.ResetOpeningText = "좋아, 깔끔하게 다 잊었어. 우리 새로 시작하자.\n안녕. 이름이 뭐야?"
	}
	if bot.ProactiveOnText == "" {
		bot.ProactiveOnText = "선톡 켰어. 타이밍 맞을 때만 먼저 톡할게."
	}
	if bot.ProactiveOffText == "" {
		bot.ProactiveOffText = "선톡 껐어. 이제 네가 먼저 말 걸 때만 답할게."
	}
	if bot.ProactiveErrorText == "" {
		bot.ProactiveErrorText = "선톡 설정하다가 잠깐 꼬였어. 한 번만 다시 쳐줘."
	}
}

func expandEnvReference(value string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return strings.TrimSpace(os.Getenv(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")))
	}
	if strings.HasPrefix(value, "env:") {
		return strings.TrimSpace(os.Getenv(strings.TrimPrefix(value, "env:")))
	}
	return value
}

func readPromptFile(baseDir string, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read persona prompt %s: %w", path, err)
	}
	return strings.TrimSpace(string(payload)), nil
}

func combinePrompts(parts ...string) string {
	combined := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			combined = append(combined, trimmed)
		}
	}
	return strings.Join(combined, "\n\n")
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

func envOllamaEndpoints() []OllamaEndpoint {
	endpoints := make([]OllamaEndpoint, 0, 4)
	for idx := 1; idx <= 20; idx++ {
		suffix := fmt.Sprintf("%02d", idx)
		baseURL := envString("OLLAMA_ENDPOINT_"+suffix+"_BASE_URL", "")
		model := envString("OLLAMA_ENDPOINT_"+suffix+"_MODEL", "")
		if baseURL == "" && model == "" {
			continue
		}
		if baseURL == "" || model == "" {
			continue
		}
		endpoints = append(endpoints, OllamaEndpoint{
			BaseURL: baseURL,
			Model:   model,
		})
	}
	return endpoints
}

func endpointBaseURLs(endpoints []OllamaEndpoint) []string {
	baseURLs := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		baseURLs = append(baseURLs, endpoint.BaseURL)
	}
	return baseURLs
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

func uniqueNonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
