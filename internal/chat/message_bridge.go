package chat

import (
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func ollamaMessagesFromConversation(history []model.Message) []ollama.Message {
	out := make([]ollama.Message, 0, len(history))
	for _, msg := range history {
		out = append(out, ollama.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return out
}
