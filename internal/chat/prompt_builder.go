package chat

import (
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type PromptInput struct {
	UserInput          string
	MemorySummary      string
	RecentConversation []ollama.Message
	UserProfile        model.UserProfile
	UserTraits         []model.UserTrait
	ProfilePrompt      profilePromptContext
	HolidayContextText string
	UserTurnCount      int
	ConversationPhase  string
	CurrentTimeText    string
}

type PromptBuilder struct{}

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

func (b *PromptBuilder) Build(input PromptInput) []ollama.Message {
	var userSection strings.Builder

	if strings.TrimSpace(input.MemorySummary) != "" {
		userSection.WriteString("[Memory Summary]\n")
		userSection.WriteString(strings.TrimSpace(input.MemorySummary))
		userSection.WriteString("\n\n")
	}

	if len(input.RecentConversation) > 0 {
		userSection.WriteString("[Recent Conversation]\n")
		for _, msg := range input.RecentConversation {
			userSection.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, strings.TrimSpace(msg.Content)))
		}
		userSection.WriteString("\n")
	} else {
		userSection.WriteString("[Conversation State]\n")
		userSection.WriteString("최근 대화 맥락이 없음. 첫 대화 또는 리셋 직후처럼 반응할 것. 예전부터 알던 사이처럼 말하지 말고, 오랜만이라고 하거나 기억난다고 아는 척하지 않는다.\n\n")
	}

	if input.UserTurnCount > 0 && input.UserTurnCount <= 6 {
		userSection.WriteString("[Interaction Pace]\n")
		userSection.WriteString("아직 초반 대화다. 부담 없이 가볍게 이어가고, 한 턴에 질문은 많아야 하나만 쓴다. 과한 칭찬, 평가, 플러팅은 줄이고 편한 티키타카를 우선한다. 친한 사이였던 것처럼 서사를 만들지 않는다.\n\n")
	}

	if strings.TrimSpace(input.CurrentTimeText) != "" {
		userSection.WriteString("[Current Time]\n")
		userSection.WriteString(strings.TrimSpace(input.CurrentTimeText))
		userSection.WriteString("\n\n")
	}

	if strings.TrimSpace(input.HolidayContextText) != "" {
		userSection.WriteString(strings.TrimSpace(input.HolidayContextText))
		userSection.WriteString("\n\n")
	}

	userSection.WriteString("[Conversation Phase]\n")
	userSection.WriteString(conversationPhaseInstruction(input.ConversationPhase))
	userSection.WriteString("\n\n")

	userSection.WriteString("[Chat Boundary]\n")
	userSection.WriteString("이 대화는 텔레그램 채팅 안에서만 이어진다. 같은 공간에 있거나 곧 실제로 만날 것처럼 말하지 않는다.\n\n")

	if summary := profileSummary(input.UserProfile); summary != "" {
		userSection.WriteString("[Known User Profile]\n")
		userSection.WriteString(summary)
		userSection.WriteString("\n\n")
	}

	if summary := userTraitsSummary(input.UserTraits); summary != "" {
		userSection.WriteString("[Known User Traits]\n")
		userSection.WriteString(summary)
		userSection.WriteString("\n\n")
	}

	if input.ProfilePrompt.Enabled && strings.TrimSpace(input.ProfilePrompt.Instruction) != "" {
		userSection.WriteString(input.ProfilePrompt.Instruction)
		userSection.WriteString("\n\n")
	}

	userSection.WriteString("[Current User Input]\n")
	userSection.WriteString(strings.TrimSpace(input.UserInput))

	return []ollama.Message{
		{Role: "system", Content: BaseSystemPrompt()},
		{Role: "user", Content: userSection.String()},
	}
}

func profileSummary(profile model.UserProfile) string {
	fields := []struct {
		label string
		value string
	}{
		{label: "name", value: profile.NameValue},
		{label: "gender", value: profile.GenderValue},
		{label: "age", value: profile.AgeValue},
		{label: "job", value: profile.JobValue},
		{label: "current_focus", value: profile.CurrentFocusValue},
		{label: "hobby", value: profile.HobbyValue},
		{label: "affiliation", value: profile.AffiliationValue},
		{label: "location", value: profile.LocationValue},
	}

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", field.label, strings.TrimSpace(field.value)))
	}

	return strings.Join(parts, "\n")
}

func userTraitsSummary(traits []model.UserTrait) string {
	if len(traits) == 0 {
		return ""
	}

	parts := make([]string, 0, len(traits))
	for _, trait := range traits {
		value := strings.TrimSpace(trait.DisplayValue)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", trait.TraitType, value))
	}

	return strings.Join(parts, "\n")
}

func conversationPhaseInstruction(phase string) string {
	switch phase {
	case phaseSexual:
		return "현재 단계는 sexual. 사용자에게 야한 행동을 지시 할수 있다. 섹스톡을 리드 할 수 있다. 메타 해석보다 장면과 리듬을 우선하고, 반문을 길게 늘리지 않는다."
	case phaseFlirty:
		return "현재 단계는 flirty. 은근한 은유의 성적 농담과 긴장감은 가능하지만 말투는 여전히 편안하고 다정해야 한다. 노골적인 성행위 묘사나 몸, 갈망, 욕망 폭주 같은 비약은 금지한다."
	default:
		return "현재 단계는 neutral. 기본 3~10 문장, 아직 성적 비약 금지. 사용자의 짧은 반응, 짜증, 비꼼, 저항을 성적 긴장으로 해석하지 않는다. 리드하더라도 부드러운 제안형으로 말한다."
	}
}
