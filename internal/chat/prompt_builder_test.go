package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestPromptBuilder_FirstConversationBlocksFalseFamiliarity(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput:     "ㅎㅇ",
		UserTurnCount: 1,
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	content := messages[1].Content
	if !strings.Contains(content, "오랜만이라고 하거나 기억난다고 아는 척하지 않는다") {
		t.Fatalf("expected first conversation guardrail, got %q", content)
	}
	if !strings.Contains(content, "한 턴에 질문은 많아야 하나만") {
		t.Fatalf("expected early-turn pace guardrail, got %q", content)
	}
	if !strings.Contains(content, "같은 공간에 있거나 곧 실제로 만날 것처럼 말하지 않는다") {
		t.Fatalf("expected chat boundary guardrail, got %q", content)
	}
}

func TestSanitizeFreshConversationReply_RemovesFalseFamiliarity(t *testing.T) {
	got := sanitizeFreshConversationReply("어, 드디어 연락했네.\n오랜만에 보는 얼굴인데 생각보다 더 좋다.\n이름이 뭐였지?")
	if strings.Contains(got, "드디어 연락") {
		t.Fatalf("expected false familiarity to be removed, got %q", got)
	}
	if strings.Contains(got, "오랜만") {
		t.Fatalf("expected false familiarity to be removed, got %q", got)
	}
	if got != "이름이 뭐였지?" {
		t.Fatalf("unexpected sanitized result: %q", got)
	}
}

func TestPromptBuilder_IncludesNeutralPhaseInstruction(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput:     "왜?",
		UserTurnCount: 3,
		ConversationStateMachine: model.ConversationStateMachine{
			TonePhase: phaseNeutral,
		},
		CurrentTimeText:    "지금 시각은 2026-04-09 15:20 KST 이다.",
		HolidayContextText: "[Holiday Context]\n. 대화 흐름에 맞을 때만 아주 짧게 스쳐가듯 반영하고, 억지 축하나 일정 추정은 하지 않는다.",
	})

	content := messages[1].Content
	if !strings.Contains(content, "[Conversation Phase]") {
		t.Fatalf("expected conversation phase section")
	}
	if !strings.Contains(content, " 성적 비약 금지") {
		t.Fatalf("expected neutral phase guardrail, got %q", content)
	}
	if !strings.Contains(content, "[Current Time]") {
		t.Fatalf("expected current time section, got %q", content)
	}
	if !strings.Contains(content, "2026-04-09 15:20 KST") {
		t.Fatalf("expected current time text, got %q", content)
	}
	if !strings.Contains(content, "[Holiday Context]") {
		t.Fatalf("expected holiday context section, got %q", content)
	}
}

func TestPromptBuilder_PrioritizesConversationStateMachinePhase(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput: "더 세게 말해줘",
		ConversationStateMachine: model.ConversationStateMachine{
			TonePhase:       phaseSexual,
			RelationalStage: "flirting",
			StageDirection:  "escalating",
			Confidence:      "high",
		},
	})

	content := messages[1].Content
	if !strings.Contains(content, "현재 단계는 sexual") {
		t.Fatalf("expected sexual phase instruction from state machine, got %q", content)
	}
}

func TestPromptBuilder_TimestampsRecentConversationAndWarnsOnHistorySummary(t *testing.T) {
	builder := NewPromptBuilder()
	recentAt := time.Date(2026, 4, 11, 13, 10, 0, 0, time.UTC)
	currentAt := time.Date(2026, 4, 12, 13, 10, 0, 0, time.UTC)

	messages := builder.Build(PromptInput{
		UserInput:            "왜 디저트 얘기야?",
		CurrentTimeText:      "지금 시각은 2026-04-12(일) 22:10 KST 이다.",
		CurrentUserInputTime: currentAt,
		HistorySummary:       "사용자가 어제 디저트 얘기를 잠깐 꺼냄.",
		RecentConversation: []model.Message{
			{Role: "assistant", Content: "오늘 고생했지", CreatedAt: recentAt},
		},
	})

	content := messages[1].Content
	if !strings.Contains(content, "최근 대화와 충돌하면 최근 대화를 우선한다") {
		t.Fatalf("expected stale history warning, got %q", content)
	}
	if !strings.Contains(content, "2026-04-11 22:10 KST assistant: 오늘 고생했지") {
		t.Fatalf("expected timestamped recent message, got %q", content)
	}
	if !strings.Contains(content, "2026-04-12 22:10 KST user: 왜 디저트 얘기야?") {
		t.Fatalf("expected timestamped current user input, got %q", content)
	}
}
