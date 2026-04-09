package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App      AppConfig
	Telegram TelegramConfig
	Ollama   OllamaConfig
	Storage  StorageConfig
	Chat     ChatConfig
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
	DefaultMode            string
	RecentTurnLimit        int
	SummaryTriggerMessages int
	SessionLockTTLSec      int
	CooldownSec            int
	ResponseMaxChars       int
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
			DefaultMode:            envString("DEFAULT_CHAT_MODE", "spicy"),
			RecentTurnLimit:        envInt("RECENT_TURN_LIMIT", 14),
			SummaryTriggerMessages: envInt("SUMMARY_TRIGGER_MESSAGES", 10),
			SessionLockTTLSec:      envInt("SESSION_LOCK_TTL_SEC", 20),
			CooldownSec:            envInt("CHAT_COOLDOWN_SEC", 2),
			ResponseMaxChars:       envInt("RESPONSE_MAX_CHARS", 0),
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
