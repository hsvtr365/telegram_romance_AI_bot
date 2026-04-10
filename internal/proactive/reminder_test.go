package proactive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

func TestWithinEventWindow_ReminderOnlyAfterDue(t *testing.T) {
	event := MemoryEvent{
		EventType: "reminder",
		EventTime: time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC),
	}

	if withinEventWindow(event, event.EventTime.Add(-1*time.Minute)) {
		t.Fatalf("reminder should not trigger before due time")
	}
	if !withinEventWindow(event, event.EventTime.Add(10*time.Minute)) {
		t.Fatalf("reminder should trigger shortly after due time")
	}
	if withinEventWindow(event, event.EventTime.Add(40*time.Minute)) {
		t.Fatalf("reminder should expire after reminder window")
	}
}

func TestReminderCandidate_HasDedicatedTriggerType(t *testing.T) {
	now := time.Date(2026, 4, 9, 9, 10, 0, 0, time.UTC)
	scanner := &Scanner{}
	session := SessionSnapshot{SessionID: 1, UserID: 2}
	event := MemoryEvent{
		EventType: "reminder",
		EventTime: time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC),
	}

	candidate, ok := scanner.reminderCandidate(session, event, now)
	if !ok {
		t.Fatalf("expected reminder candidate")
	}
	if candidate.TriggerType != TriggerReminder {
		t.Fatalf("expected reminder trigger, got %s", candidate.TriggerType)
	}
	if candidate.Priority != 120 {
		t.Fatalf("expected reminder priority 120, got %d", candidate.Priority)
	}
}

func TestDefaultSeedLibrary_PicksReminderSpecificSeed(t *testing.T) {
	library := NewDefaultSeedLibrary()
	seed := library.Pick(
		TriggerCandidate{TriggerType: TriggerReminder},
		Strategy{Purpose: PurposeFollowup, Tone: ToneSoft, Intensity: IntensityMedium},
		ProactiveProfile{},
	)

	if seed.Key == "fallback" {
		t.Fatalf("expected reminder-specific seed, got fallback")
	}
}

func TestComposer_UsesReminderLLMForReminder(t *testing.T) {
	cfg := DefaultConfig()
	mainLLM := &stubLLM{reply: "메인"}
	reminderLLM := &stubLLM{reply: "지금 알림할 시간이야."}
	composer := NewComposer(cfg, mainLLM, reminderLLM, nil, nil)

	result, err := composer.Compose(context.Background(), ComposeInput{
		Session:   SessionSnapshot{Mode: "soft"},
		Candidate: TriggerCandidate{TriggerType: TriggerReminder, TriggerRefID: "reminder:1:1", CandidateID: "reminder:1:1"},
		Strategy: Strategy{
			Type:      "reminder",
			Intensity: IntensityMedium,
			Tone:      ToneSoft,
			Purpose:   PurposeFollowup,
			Length:    LengthShort,
		},
	})
	if err != nil {
		t.Fatalf("compose should not fail: %v", err)
	}
	if result.Message != "지금 알림할 시간이야." {
		t.Fatalf("expected reminder llm output, got %q", result.Message)
	}
	if len(mainLLM.calls) != 0 {
		t.Fatalf("expected main llm to be unused for reminder")
	}
	if len(reminderLLM.calls) != 1 {
		t.Fatalf("expected reminder llm to be used once")
	}
}

func TestPromptBuilder_ReminderPromptBlocksGenericProactiveTone(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		Candidate: TriggerCandidate{TriggerType: TriggerReminder, TriggerRefID: "reminder:1:1"},
		Strategy:  Strategy{Purpose: PurposeFollowup, Tone: ToneSoft, Intensity: IntensityMedium, Length: LengthShort},
		Seed:      Seed{Text: "약속한 시간이라 알려주러 왔어."},
	})

	if len(messages) != 2 {
		t.Fatalf("expected system and user messages")
	}
	if got := messages[0].Content; !containsAll(got, []string{"리마인드 톡 한 줄 작성기", "질문하지 않는다", "\"보고 싶어서\""}) {
		t.Fatalf("unexpected reminder system prompt: %q", got)
	}
}

func TestPreparedReminderComposeResult_UsesStoredPreparedMessage(t *testing.T) {
	event := &MemoryEvent{
		EventType:    "reminder",
		EventSubtype: "oneoff",
		EventValue: map[string]any{
			"prepared_message": "말해달라고 한 시간이라 왔어.",
		},
	}

	result, ok := preparedReminderComposeResult(event)
	if !ok {
		t.Fatalf("expected prepared reminder compose result")
	}
	if result.Message != "말해달라고 한 시간이라 왔어." {
		t.Fatalf("unexpected prepared reminder message: %q", result.Message)
	}
}

type stubLLM struct {
	reply string
	calls [][]ollama.Message
}

func (s *stubLLM) Chat(_ context.Context, messages []ollama.Message) (string, error) {
	s.calls = append(s.calls, messages)
	return s.reply, nil
}

func containsAll(text string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}
