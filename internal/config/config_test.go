package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ReadsBotsConfig(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "TELEGRAM_BOTS_CONFIG_PATH=bots.json\n")
	writeFile(t, filepath.Join(dir, "common.md"), "common rules")
	writeFile(t, filepath.Join(dir, "persona-one.md"), "persona one")
	writeFile(t, filepath.Join(dir, "persona-two.md"), "persona two")
	writeFile(t, filepath.Join(dir, "bots.json"), `{
  "common_prompt_path": "common.md",
  "bots": [
    {
      "id": "one",
      "name": "One",
      "telegram_bot_token": "token-one",
      "persona_prompt_path": "persona-one.md",
      "default_chat_mode": "soft",
      "welcome_text": "hello one",
      "fallback_text": "fallback one",
      "reset_text": "reset one"
    },
    {
      "id": "two",
      "name": "Two",
      "telegram_bot_token": "token-two",
      "persona_prompt_path": "persona-two.md",
      "default_chat_mode": "spicy",
      "welcome_text": "hello two",
      "fallback_text": "fallback two",
      "reset_text": "reset two"
    }
  ]
}`)

	cfg, err := Load(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Bots) != 2 {
		t.Fatalf("expected 2 bots, got %d", len(cfg.Bots))
	}
	if cfg.Bots[0].PersonaPrompt != "common rules\n\npersona one" {
		t.Fatalf("unexpected first persona prompt: %q", cfg.Bots[0].PersonaPrompt)
	}
	if cfg.Bots[1].PersonaPrompt != "common rules\n\npersona two" {
		t.Fatalf("unexpected second persona prompt: %q", cfg.Bots[1].PersonaPrompt)
	}
	if cfg.Bots[1].WelcomeText != "hello two" {
		t.Fatalf("unexpected second welcome text: %q", cfg.Bots[1].WelcomeText)
	}
	if len(cfg.Bots[0].Channels) != 1 || cfg.Bots[0].Channels[0].Type != "telegram" || cfg.Bots[0].Channels[0].Token != "token-one" {
		t.Fatalf("expected legacy telegram token to become telegram channel, got %+v", cfg.Bots[0].Channels)
	}
}

func TestLoad_ReadsExplicitChannelConfig(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "TELEGRAM_BOTS_CONFIG_PATH=bots.json\nLEE_TOKEN=token-lee\n")
	writeFile(t, filepath.Join(dir, "persona.md"), "persona")
	writeFile(t, filepath.Join(dir, "bots.json"), `{
  "bots": [
    {
      "id": "lee-jinhyuk",
      "name": "Lee",
      "persona_prompt_path": "persona.md",
      "channels": [
        {"type": "telegram", "token": "${LEE_TOKEN}"}
      ]
    }
  ]
}`)

	cfg, err := Load(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Bots[0].Channels[0].Token; got != "token-lee" {
		t.Fatalf("unexpected channel token: %q", got)
	}
	if got := cfg.Bots[0].TelegramBotToken; got != "token-lee" {
		t.Fatalf("expected telegram token compatibility field to be populated, got %q", got)
	}
}

func TestValidateRejectsDuplicateBotID(t *testing.T) {
	cfg := Config{
		Bots: []BotConfig{
			{ID: "same", TelegramBotToken: "token-one", PersonaPromptPath: "one.md", PersonaPrompt: "persona one"},
			{ID: "same", TelegramBotToken: "token-two", PersonaPromptPath: "two.md", PersonaPrompt: "persona two"},
		},
		Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "model"},
		Storage: StorageConfig{PostgresDSN: "postgres://example", RedisURL: "redis://example"},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate bot id") {
		t.Fatalf("expected duplicate bot id validation error, got %v", err)
	}
}

func TestLoadFallsBackToLegacyTelegramBotToken(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "TELEGRAM_BOT_TOKEN=legacy-token\nTELEGRAM_COMMON_PROMPT_PATH=common.md\nTELEGRAM_PERSONA_PROMPT_PATH=persona.md\n")
	writeFile(t, filepath.Join(dir, "common.md"), "legacy common")
	writeFile(t, filepath.Join(dir, "persona.md"), "legacy persona")

	cfg, err := Load(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Bots) != 1 {
		t.Fatalf("expected one legacy bot, got %d", len(cfg.Bots))
	}
	if got := cfg.Bots[0].TelegramBotToken; got != "legacy-token" {
		t.Fatalf("unexpected legacy token: %q", got)
	}
	if got := cfg.Bots[0].PersonaPrompt; got != "legacy common\n\nlegacy persona" {
		t.Fatalf("unexpected legacy persona: %q", got)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"TELEGRAM_BOTS_CONFIG_PATH",
		"TELEGRAM_BOT_TOKEN",
		"TELEGRAM_COMMON_PROMPT_PATH",
		"TELEGRAM_PERSONA_PROMPT_PATH",
		"LEE_TOKEN",
	}
	original := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		original[key] = value
		present[key] = ok
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, original[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
