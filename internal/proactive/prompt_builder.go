package proactive

import (
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type PromptInput struct {
	Candidate          TriggerCandidate
	Strategy           Strategy
	Seed               Seed
	MemorySummary      string
	RelationshipNote   string
	EventNote          string
	HolidayContextText string
	RecentConversation []ollama.Message
}

type PromptBuilder struct{}

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

func (b *PromptBuilder) Build(input PromptInput) []ollama.Message {
	if input.Candidate.TriggerType == TriggerReminder {
		return b.buildReminder(input)
	}

	var userSection strings.Builder

	userSection.WriteString("[Proactive Context]\n")
	userSection.WriteString(fmt.Sprintf("trigger_type: %s\n", input.Candidate.TriggerType))
	userSection.WriteString(fmt.Sprintf("trigger_ref_id: %s\n", input.Candidate.TriggerRefID))
	userSection.WriteString(fmt.Sprintf("purpose: %s\n", input.Strategy.Purpose))
	userSection.WriteString(fmt.Sprintf("tone: %s\n", input.Strategy.Tone))
	userSection.WriteString(fmt.Sprintf("intensity: %s\n", input.Strategy.Intensity))
	userSection.WriteString(fmt.Sprintf("length: %s\n", input.Strategy.Length))
	userSection.WriteString(fmt.Sprintf("seed: %s\n", strings.TrimSpace(input.Seed.Text)))
	userSection.WriteString("\n")

	if strings.TrimSpace(input.RelationshipNote) != "" {
		userSection.WriteString("[Relationship Note]\n")
		userSection.WriteString(strings.TrimSpace(input.RelationshipNote))
		userSection.WriteString("\n\n")
	}

	if strings.TrimSpace(input.EventNote) != "" {
		userSection.WriteString("[Event Note]\n")
		userSection.WriteString(strings.TrimSpace(input.EventNote))
		userSection.WriteString("\n\n")
	}

	if strings.TrimSpace(input.MemorySummary) != "" {
		userSection.WriteString("[Memory Summary]\n")
		userSection.WriteString(strings.TrimSpace(input.MemorySummary))
		userSection.WriteString("\n\n")
	}

	if strings.TrimSpace(input.HolidayContextText) != "" {
		userSection.WriteString(strings.TrimSpace(input.HolidayContextText))
		userSection.WriteString("\n\n")
	}

	if len(input.RecentConversation) > 0 {
		userSection.WriteString("[Recent Conversation]\n")
		for _, msg := range input.RecentConversation {
			userSection.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, strings.TrimSpace(msg.Content)))
		}
		userSection.WriteString("\n")
	}

	systemPrompt := chat.BaseSystemPrompt() + "\n\n" + strings.TrimSpace(`
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

	userSection.WriteString("[Reminder Context]\n")
	userSection.WriteString(fmt.Sprintf("trigger_ref_id: %s\n", input.Candidate.TriggerRefID))
	userSection.WriteString(fmt.Sprintf("seed: %s\n", strings.TrimSpace(input.Seed.Text)))
	if strings.TrimSpace(input.EventNote) != "" {
		userSection.WriteString(fmt.Sprintf("event_note: %s\n", strings.TrimSpace(input.EventNote)))
	}
	if len(input.RecentConversation) > 0 {
		last := input.RecentConversation[len(input.RecentConversation)-1]
		userSection.WriteString(fmt.Sprintf("latest_message: %s\n", strings.TrimSpace(last.Content)))
	}

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
