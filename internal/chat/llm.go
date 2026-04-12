package chat

import (
	"context"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type LLM interface {
	Chat(ctx context.Context, messages []ollama.Message) (string, error)
}
