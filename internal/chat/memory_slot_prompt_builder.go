package chat

import (
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type MemorySlotAnalyzeInput struct {
	Snapshot         model.MemorySlotSnapshot
	CurrentUserInput string
}

type MemorySlotPromptBuilder struct{}

func NewMemorySlotPromptBuilder() *MemorySlotPromptBuilder {
	return &MemorySlotPromptBuilder{}
}

func (b *MemorySlotPromptBuilder) Build(input MemorySlotAnalyzeInput) []ollama.Message {
	var userSection strings.Builder

	if len(input.Snapshot.TopicSlots) > 0 {
		lines := make([]string, 0, len(input.Snapshot.TopicSlots))
		for _, slot := range input.Snapshot.TopicSlots {
			lines = append(lines, fmt.Sprintf(
				"slot_key=%s | label=%s | status=%s | importance=%d | confidence=%s | mention_count=%d | summary=%s | last_seen_at=%s",
				slot.SlotKey,
				strings.TrimSpace(slot.TopicLabel),
				slot.Status,
				slot.Importance,
				normalizeConfidence(slot.Confidence),
				slot.MentionCount,
				strings.TrimSpace(slot.Summary),
				promptutil.FormatPromptTimestamp(slot.LastSeenAt),
			))
		}
		promptutil.WriteLinesSection(&userSection, "Current Topic Slots", lines)
	}

	if len(input.Snapshot.RecentMessages) > 0 {
		messages := make([]promptutil.MessageLine, 0, len(input.Snapshot.RecentMessages))
		for _, msg := range input.Snapshot.RecentMessages {
			messages = append(messages, promptutil.MessageLine{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
		}
		promptutil.WriteConversation(&userSection, "Recent Conversation", messages)
	}

	promptutil.WriteSection(&userSection, "Current User Input", input.CurrentUserInput)

	systemPrompt := strings.TrimSpace(`
You analyze Korean chat context and maintain memory slots for a chatbot.
Return one JSON object only. No markdown, no prose.
Be aggressive about detecting important ongoing topics and topic transitions, but do not invent concrete facts that are unsupported.

Rules:
- topics should capture important ongoing or emotionally meaningful conversation themes.
- A topic may be created from a single strong user message.
- Use status: active | watch | resolved | archived.
- Use confidence: high | medium | low.
- importance: integer 1-100.
- evidence_texts should be short direct snippets from the conversation.
- resolved_topics should list labels of previously ongoing topics that now look completed or no longer active.

Schema:
{
  "topics":[
    {
      "label":"",
      "summary":"",
      "status":"active",
      "importance":80,
      "confidence":"medium",
      "evidence_texts":[""],
      "supersedes":[""]
    }
  ],
  "resolved_topics":[""]
}

You MUST always return the complete JSON structure above exactly as shown, even if no facts or topics are found (leave arrays empty and properties as defaults). Do not output an empty response or plain text.
`)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}
