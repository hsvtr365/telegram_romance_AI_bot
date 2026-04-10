package chat

import (
	"context"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

func TestPrepareReminderMessage_UsesReminderLLMResult(t *testing.T) {
	service := &Service{
		reminderLLM: reminderStubLLM{reply: "약속한 시간이라 바로 알려줘."},
	}

	text, source := service.prepareReminderMessage(context.Background(), "1분 뒤 알려줘", time.Date(2026, 4, 10, 9, 38, 0, 0, time.UTC), time.Date(2026, 4, 10, 9, 37, 0, 0, time.UTC))
	if source != "generated" {
		t.Fatalf("expected generated source, got %s", source)
	}
	if text != "약속한 시간이라 바로 알려줘." {
		t.Fatalf("unexpected generated reminder text: %q", text)
	}
}

func TestPrepareReminderMessage_FallsBackWhenReminderLLMUnavailable(t *testing.T) {
	service := &Service{}

	text, source := service.prepareReminderMessage(context.Background(), "1분 뒤 알려줘", time.Date(2026, 4, 10, 9, 38, 0, 0, time.UTC), time.Date(2026, 4, 10, 9, 37, 0, 0, time.UTC))
	if source != "fallback" {
		t.Fatalf("expected fallback source, got %s", source)
	}
	if text == "" {
		t.Fatalf("expected fallback reminder text")
	}
}

type reminderStubLLM struct {
	reply string
}

func (s reminderStubLLM) Chat(_ context.Context, _ []ollama.Message) (string, error) {
	return s.reply, nil
}
