package proactive

import (
	"strings"
	"testing"
)

func TestPromptBuilder_IncludesHolidayContext(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		Candidate: TriggerCandidate{TriggerType: TriggerReconnect, TriggerRefID: "reconnect:1"},
		Strategy: Strategy{
			Purpose:   PurposeCheckin,
			Tone:      ToneSoft,
			Intensity: IntensityLight,
			Length:    LengthShort,
		},
		Seed:               Seed{Text: "뭐 해"},
		HolidayContextText: "[Holiday Context]\n추석까지 3일 남았다. 대화와 맞을 때만 한 번 가볍게 언급할 수 있고, 반복하거나 과장하지 않는다.",
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if !strings.Contains(messages[1].Content, "[Holiday Context]") {
		t.Fatalf("expected holiday context in prompt, got %q", messages[1].Content)
	}
}

func TestPromptBuilder_IncludesMemorySummary(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		Candidate:     TriggerCandidate{TriggerType: TriggerReconnect, TriggerRefID: "reconnect:1"},
		Strategy:      Strategy{Purpose: PurposeCheckin, Tone: ToneSoft, Intensity: IntensityLight, Length: LengthShort},
		Seed:          Seed{Text: "뭐 해"},
		MemorySummary: "[Memory Summary]\n- topic: 시험 준비 | 아직 계획을 세우는 중\n[Conversation State]\n- stage=support",
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if !strings.Contains(messages[1].Content, "[Memory Summary]") {
		t.Fatalf("expected memory summary in prompt, got %q", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, "시험 준비") {
		t.Fatalf("expected memory summary text in prompt, got %q", messages[1].Content)
	}
}
