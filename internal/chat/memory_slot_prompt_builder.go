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
				"slot_key=%s | label=%s | status=%s | importance=%d | confidence=%s | mention_count=%d | summary=%s",
				slot.SlotKey,
				strings.TrimSpace(slot.TopicLabel),
				slot.Status,
				slot.Importance,
				normalizeConfidence(slot.Confidence),
				slot.MentionCount,
				strings.TrimSpace(slot.Summary),
			))
		}
		promptutil.WriteLinesSection(&userSection, "Current Topic Slots", lines)
	}

	if state := input.Snapshot.ConversationState; state.SessionID != 0 || state.CurrentStage != "" || state.StageDirection != "" || state.EmotionalTone != "" || state.OpenLoopSummary != "" {
		lines := make([]string, 0, 6)
		if strings.TrimSpace(state.CurrentStage) != "" {
			lines = append(lines, "current_stage="+strings.TrimSpace(state.CurrentStage))
		}
		if strings.TrimSpace(state.StageDirection) != "" {
			lines = append(lines, "stage_direction="+strings.TrimSpace(state.StageDirection))
		}
		if strings.TrimSpace(state.EmotionalTone) != "" {
			lines = append(lines, "emotional_tone="+strings.TrimSpace(state.EmotionalTone))
		}
		if strings.TrimSpace(state.InteractionMode) != "" {
			lines = append(lines, "interaction_mode="+strings.TrimSpace(state.InteractionMode))
		}
		if strings.TrimSpace(state.OpenLoopSummary) != "" {
			lines = append(lines, "open_loop_summary="+strings.TrimSpace(state.OpenLoopSummary))
		}
		if strings.TrimSpace(state.FocusTopicKey) != "" {
			lines = append(lines, "focus_topic_key="+strings.TrimSpace(state.FocusTopicKey))
		}
		promptutil.WriteLinesSection(&userSection, "Current Conversation State", lines)
	}

	if len(input.Snapshot.RecentMessages) > 0 {
		messages := make([]promptutil.MessageLine, 0, len(input.Snapshot.RecentMessages))
		for _, msg := range input.Snapshot.RecentMessages {
			messages = append(messages, promptutil.MessageLine{Role: msg.Role, Content: msg.Content})
		}
		promptutil.WriteConversation(&userSection, "Recent Conversation", messages)
	}

	promptutil.WriteSection(&userSection, "Current User Input", input.CurrentUserInput)

	systemPrompt := strings.TrimSpace(`
You analyze Korean chat context and maintain memory slots for a chatbot.
Return one JSON object only. No markdown, no prose.
Be aggressive about detecting important ongoing topics and relationship-state shifts, but do not invent concrete facts that are unsupported.

Rules:
- topics should capture important ongoing or emotionally meaningful conversation themes.
- A topic may be created from a single strong user message.
- Use status: active | watch | resolved | archived.
- Use confidence: high | medium | low.
- importance: integer 1-100.
- current_stage must be one of: opener | rapport | flirting | support | conflict | repair | planning.
- stage_direction must be one of: warming | stable | escalating | cooling | shifting.
- focus_topic_label should match one topic label when possible, or empty string.
- evidence_texts should be short direct snippets from the conversation.
- resolved_topics should list labels of previously ongoing topics that now look completed or no longer active.
- supersedes should list older topic labels that should be absorbed into this topic.

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
  "conversation_state":{
    "current_stage":"rapport",
    "stage_direction":"warming",
    "emotional_tone":"",
    "interaction_mode":"",
    "open_loop_summary":"",
    "focus_topic_label":"",
    "confidence":"medium",
    "evidence_texts":[""]
  },
  "resolved_topics":[""]
}
`)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}
