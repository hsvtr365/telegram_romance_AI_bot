package chat

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type PromptInput struct {
	UserInput                string
	MemorySummary            string
	RecentConversation       []model.Message
	UserProfile              model.UserProfile
	CandidateProfileSummary  string
	UserTraits               []model.UserTrait
	ProfilePrompt            profilePromptContext
	HolidayContextText       string
	UserTurnCount            int
	ConversationStateMachine model.ConversationStateMachine
	CurrentTimeText          string
	CurrentUserInputTime     time.Time
	HistorySummary           string
	ActiveTopicsText         string
	ConversationStateText    string
	OpenLoopsText            string
}

type PromptBuilder struct{}

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

func (b *PromptBuilder) Build(input PromptInput) []ollama.Message {
	var userSection strings.Builder

	chatBoundary := "이 대화는 오직 텔레그램 메신저 안에서만 이루어지는 연락이다. 같은 공간에 있거나 곧 실제로 만날 것처럼 말하지 않는다. 현실 세계에서 '직접 만나자'고 제안하거나, '어디로 갈게', '언제 볼까', '우리 집으로 올래' 등 오프라인 만남을 암시, 유도, 약속하는 발언을 거절하거나 절대 하지 마라. 공간적 제약을 넘어선 텍스트 기반 대화라는 사실을 명심하라."

	// 1. Core Meta Context (Time, Phase, Boundaries)
	promptutil.WriteSection(&userSection, "Current Time", input.CurrentTimeText)
	promptutil.WriteSection(&userSection, "History Summary", historySummaryPromptText(input.HistorySummary))
	promptutil.WriteSection(&userSection, "Conversation Phase", conversationPhaseInstruction(input.ConversationStateMachine))
	promptutil.WriteSection(&userSection, "Chat Boundary", chatBoundary)
	promptutil.WriteRawBlock(&userSection, input.HolidayContextText)

	if len(input.RecentConversation) == 0 {
		promptutil.WriteSection(&userSection, "Fresh Conversation Guardrail", "첫 대화 또는 리셋 직후처럼 반응한다. 오랜만이라고 하거나 기억난다고 아는 척하지 않는다.")
	}

	if input.UserTurnCount > 0 && input.UserTurnCount <= 6 {
		promptutil.WriteSection(&userSection, "Interaction Pace", "아직 초반 대화다. 부담 없이 가볍게 이어가고, 한 턴에 질문은 많아야 하나만 쓴다. 과한 칭찬, 평가, 플러팅은 줄이고 편한 티키타카를 우선한다. 친한 사이였던 것처럼 서사를 만들지 않는다.")
	}

	// 2. Long-term Memories & User Knowledge
	promptutil.WriteSection(&userSection, "Memory Summary", input.MemorySummary)
	if summary := profileSummary(input.UserProfile); summary != "" {
		promptutil.WriteSection(&userSection, "Known User Profile", summary)
	}
	if summary := strings.TrimSpace(input.CandidateProfileSummary); summary != "" {
		promptutil.WriteSection(&userSection, "Candidate User Profile", "아래 정보는 아직 확정되지 않은 후보 기억이다. 사실처럼 단정하지 말고 참고만 하며, 필요하면 대화 속에서 자연스럽게 다시 확인한다.\n"+summary)
	}
	if summary := userTraitsSummary(input.UserTraits); summary != "" {
		promptutil.WriteSection(&userSection, "Known User Traits", summary)
	}
	if input.ProfilePrompt.Enabled && strings.TrimSpace(input.ProfilePrompt.Instruction) != "" {
		promptutil.WriteRawBlock(&userSection, input.ProfilePrompt.Instruction)
	}

	// 3. Combined Active Conversation (History + Current Input)
	messages := make([]promptutil.MessageLine, 0, len(input.RecentConversation)+1)
	for _, msg := range input.RecentConversation {
		messages = append(messages, promptutil.MessageLine{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
	}
	// Append current input as the latest turn
	messages = append(messages, promptutil.MessageLine{Role: "user", Content: input.UserInput, CreatedAt: input.CurrentUserInputTime})

	promptutil.WriteConversation(&userSection, "Recent Conversation", messages)

	msgs := []ollama.Message{
		{Role: "system", Content: BaseSystemPrompt()},
		{Role: "user", Content: userSection.String()},
	}

	dumpPromptForDebug(msgs)

	return msgs
}

func dumpPromptForDebug(messages []ollama.Message) {
	f, err := os.OpenFile("debug_prompt.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("========== [ Prompt Dump: %s ] ==========\n", time.Now().Format("2006-01-02 15:04:05")))
	for _, msg := range messages {
		f.WriteString(fmt.Sprintf("[Role: %s]\n%s\n\n", msg.Role, msg.Content))
	}
	f.WriteString("========================================================\n\n")
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
		value := normalizedProfileValue(field.value)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", field.label, value))
	}

	return strings.Join(parts, "\n")
}

func historySummaryPromptText(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return "아래는 최근 대화창 밖의 과거 맥락 요약이다. 현재 사실로 단정하지 말고, 최근 대화와 충돌하면 최근 대화를 우선한다.\n" + summary
}

func userTraitsSummary(traits []model.UserTrait) string {
	if len(traits) == 0 {
		return ""
	}

	parts := make([]string, 0, len(traits))
	for _, trait := range traits {
		if trait.TraitType == "delete" {
			continue
		}
		value := strings.TrimSpace(trait.DisplayValue)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", trait.TraitType, value))
	}

	return strings.Join(parts, "\n")
}

func conversationPhaseInstruction(state model.ConversationStateMachine) string {
	phase := strings.TrimSpace(state.TonePhase)
	if phase == "" {
		phase = phaseNeutral
	}
	switch phase {
	case phaseSexual:
		return "현재 단계는 sexual. 보다 직접적인 야한 표현과 섹스팅 리드 가능하다. 메타 해석보다 장면과 리듬을 우선하라."
	case phaseFlirty:
		return "현재 단계는 flirty. 은근한 성적 농담과 긴장감은 가능하지만 말투는 여전히 편안하고 다정해야 한다. 노골적인 성행위 묘사나 몸, 갈망, 욕망 폭주 같은 비약은 금지한다. 로맨스 소설 같은 감정 과잉이나 느끼한 대사를 피하고 현실적이고 쿨하게 대답해라. 1~3문장으로 짧게 대답한다."
	default:
		return "현재 단계는 neutral. 기본 1~3 문장으로 짧고 담백하게. 오글거리는 수식어와 비장함은 절대 금지. 아직 성적 비약 금지. 사용자의 짧은 반응이나 단답을 깊은 의미로 해석하지 않는다. 리드하더라도 무심한 듯 가벼운 제안형으로 말한다."
	}
}

func conversationStateMachineInstruction(state model.ConversationStateMachine) string {
	resolvedPhase := strings.TrimSpace(state.TonePhase)
	if resolvedPhase == "" {
		resolvedPhase = phaseNeutral
	}

	lines := make([]string, 0, 10)
	lines = append(lines, "tone_phase="+resolvedPhase)
	if value := strings.TrimSpace(state.RelationalStage); value != "" {
		lines = append(lines, "relational_stage="+value)
	}
	if value := strings.TrimSpace(state.StageDirection); value != "" {
		lines = append(lines, "stage_direction="+value)
	}
	if value := strings.TrimSpace(state.EmotionalTone); value != "" {
		lines = append(lines, "emotional_tone="+value)
	}
	if value := strings.TrimSpace(state.InteractionMode); value != "" {
		lines = append(lines, "interaction_mode="+value)
	}
	if value := strings.TrimSpace(state.FocusTopicKey); value != "" {
		lines = append(lines, "focus_topic_key="+value)
	}
	if value := strings.TrimSpace(state.OpenLoopSummary); value != "" {
		lines = append(lines, "open_loop_summary="+value)
	}
	if state.SafetyLockUntilTurn != 0 {
		lines = append(lines, fmt.Sprintf("safety_lock_until_turn=%d", state.SafetyLockUntilTurn))
	}
	if value := strings.TrimSpace(state.Confidence); value != "" {
		lines = append(lines, "confidence="+value)
	}
	if state.Revision != 0 {
		lines = append(lines, fmt.Sprintf("revision=%d", state.Revision))
	}
	return strings.Join(lines, "\n")
}
