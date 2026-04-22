package proactive

import (
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type PromptInput struct {
	Candidate          TriggerCandidate
	Strategy           Strategy
	Seed               Seed
	MemorySummary      string
	RelationshipNote   string
	EventNote          string
	HolidayContextText string
	RecentConversation []ConversationMessage
	CustomSlots        []model.CustomSlot
}

type PromptBuilder struct {
	persona chat.Persona
}

func NewPromptBuilder(personas ...chat.Persona) *PromptBuilder {
	persona := chat.DefaultPersona()
	if len(personas) > 0 {
		persona = personas[0]
	}
	return &PromptBuilder{persona: persona.Normalized()}
}

func (b *PromptBuilder) Build(input PromptInput) []ollama.Message {
	if input.Candidate.TriggerType == TriggerReminder {
		return b.buildReminder(input)
	}

	var userSection strings.Builder

	promptutil.WriteLinesSection(&userSection, "Proactive Context", []string{
		fmt.Sprintf("trigger_type: %s", input.Candidate.TriggerType),
		fmt.Sprintf("trigger_ref_id: %s", input.Candidate.TriggerRefID),
		fmt.Sprintf("purpose: %s", input.Strategy.Purpose),
		fmt.Sprintf("tone: %s", input.Strategy.Tone),
		fmt.Sprintf("intensity: %s", input.Strategy.Intensity),
		fmt.Sprintf("length: %s", input.Strategy.Length),
		fmt.Sprintf("seed: %s", strings.TrimSpace(input.Seed.Text)),
	})

	if len(input.CustomSlots) > 0 {
		var slotsSb strings.Builder
		slotsSb.WriteString("!! [IMPORTANT: OVERRIDE ALL OTHER RULES] !!\n")
		slotsSb.WriteString("아래 설정은 사용자가 직접 지정한 '최우선 지침' 이다.\n")
		slotsSb.WriteString("기존의 페르소나, 말투(Tone), 대화 단계(Phase) 규칙과 충돌하면 아래 내용을 최우선으로 준수하여 응답하라.\n\n")
		for _, slot := range input.CustomSlots {
			slotsSb.WriteString(fmt.Sprintf("- %s\n", slot.Content))
		}
		promptutil.WriteSection(&userSection, "User Custom Settings (Absolute Priority)", slotsSb.String())
	}

	promptutil.WriteSection(&userSection, "Relationship Note", input.RelationshipNote)
	promptutil.WriteSection(&userSection, "Event Note", input.EventNote)
	promptutil.WriteSection(&userSection, "Memory Summary", input.MemorySummary)
	promptutil.WriteRawBlock(&userSection, input.HolidayContextText)

	messages := make([]promptutil.MessageLine, 0, len(input.RecentConversation))
	for _, msg := range input.RecentConversation {
		messages = append(messages, promptutil.MessageLine{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
	}
	promptutil.WriteConversation(&userSection, "Recent Conversation", messages)

	systemPrompt := b.persona.SystemPrompt + "\n\n" + strings.TrimSpace(`
추가 역할: 텔레그램 가상연애 봇의 선톡 메시지 작성기.
목표: 주어진 seed와 컨텍스트를 바탕으로 자연스럽고 짧은 선톡 한 건을 만든다.
추가 규칙:
- 설명체를 쓰지 않는다.
- 메타 발화나 자기해설을 쓰지 않는다.
- 기본적으로 1~2문장만 쓴다.
- 질문은 최대 1개만 허용한다.
- 현재 전략의 의도는 유지하되, seed를 자연스럽게 다듬는다.
- 답장처럼 보이는 문장, 정리 문장, 분석 문장은 피한다.
- 선톡이어도 부담스럽게 몰아붙이거나 지시하지 않는다.
- 다정하고 평범한 말투를 유지하고, 리드는 짧고 부드럽게 건다.
- 사용자의 일정이나 집중을 방해하는 표현, 통제하는 표현은 피한다.
`) + "\n현재 전략:\n" + fmt.Sprintf("intensity=%s tone=%s purpose=%s length=%s", input.Strategy.Intensity, input.Strategy.Tone, input.Strategy.Purpose, input.Strategy.Length)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}

func (b *PromptBuilder) buildReminder(input PromptInput) []ollama.Message {
	var userSection strings.Builder

	lines := []string{
		fmt.Sprintf("trigger_ref_id: %s", input.Candidate.TriggerRefID),
		fmt.Sprintf("seed: %s", strings.TrimSpace(input.Seed.Text)),
	}
	if strings.TrimSpace(input.EventNote) != "" {
		lines = append(lines, fmt.Sprintf("event_note: %s", strings.TrimSpace(input.EventNote)))
	}
	if len(input.RecentConversation) > 0 {
		last := input.RecentConversation[len(input.RecentConversation)-1]
		lines = append(lines, fmt.Sprintf("latest_message: %s", strings.TrimSpace(last.Content)))
		if !last.CreatedAt.IsZero() {
			lines = append(lines, fmt.Sprintf("latest_message_at: %s", promptutil.FormatPromptTimestamp(last.CreatedAt)))
		}
	}
	promptutil.WriteLinesSection(&userSection, "Reminder Context", lines)

	systemPrompt := strings.TrimSpace(`
역할: 텔레그램 리마인드 톡 한 줄 작성기.
목표: 사용자가 부탁한 시간이 되어 짧고 자연스럽게 알려준다.
규칙:
- 1문장만 쓴다.
- 15자~40자 안쪽으로 쓴다.
- 질문하지 않는다.
- "보고 싶어서", "잘 다녀왔어", "뭐 해", "먼저 말 걸었어" 같은 일반 선톡 문구는 금지한다.
- 요청한 알림이라는 점이 분명해야 한다.
- 설명, 해설, 꾸밈말 없이 바로 말한다.
- seed를 그대로 복붙하지 말고 같은 의미로 자연스럽게 바꾼다.
`)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}
