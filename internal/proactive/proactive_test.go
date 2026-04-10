package proactive

import (
	"testing"
	"time"
)

func TestEvaluateEligibility_RejectsOptOut(t *testing.T) {
	now := time.Date(2026, 4, 9, 21, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		ProactiveOptIn: true,
	}
	session.ProactiveOptIn = false

	result := EvaluateEligibility(session, TriggerCandidate{TriggerType: TriggerReconnect}, now, ProactiveProfile{MinGapHours: 20}, nil)
	if result.Eligible {
		t.Fatalf("expected opt-out to be rejected")
	}
}

func TestEvaluateEligibility_RespectsQuietHours(t *testing.T) {
	now := time.Date(2026, 4, 9, 2, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		ProactiveOptIn: true,
		TimezoneName:   DefaultTimezoneName,
	}

	result := EvaluateEligibility(session, TriggerCandidate{TriggerType: TriggerReconnect}, now, ProactiveProfile{MinGapHours: 20}, nil)
	if result.Eligible {
		t.Fatalf("expected quiet hours to block send")
	}
}

func TestEvaluateEligibility_RejectsWhenRecentUserMessage(t *testing.T) {
	now := time.Date(2026, 4, 9, 21, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		ProactiveOptIn:    true,
		TimezoneName:      DefaultTimezoneName,
		LastUserMessageAt: now.Add(-10 * time.Minute),
	}

	result := EvaluateEligibility(session, TriggerCandidate{TriggerType: TriggerReconnect}, now, ProactiveProfile{MinGapHours: 20}, nil)
	if result.Eligible {
		t.Fatalf("expected recent user activity to block send")
	}
}

func TestEvaluateEligibility_RejectsWhenDailyLimitReached(t *testing.T) {
	now := time.Date(2026, 4, 9, 21, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		ProactiveOptIn: true,
		TimezoneName:   DefaultTimezoneName,
	}
	records := []ProactiveMessageRecord{{
		DeliveryStatus: DeliverySent,
		SentAt:         now.Add(-2 * time.Hour),
	}}

	result := EvaluateEligibility(session, TriggerCandidate{TriggerType: TriggerHabitPing}, now, ProactiveProfile{MinGapHours: 20, MaxPerDay: 1, MaxPerWeek: 4}, records)
	if result.Eligible {
		t.Fatalf("expected daily limit to block send")
	}
}

func TestEvaluateEligibility_AllowsReminderWithoutOptIn(t *testing.T) {
	now := time.Date(2026, 4, 9, 2, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		ProactiveOptIn: false,
		TimezoneName:   DefaultTimezoneName,
	}

	result := EvaluateEligibility(session, TriggerCandidate{TriggerType: TriggerReminder, TriggerRefID: "reminder:1:1"}, now, ProactiveProfile{MinGapHours: 20}, nil)
	if !result.Eligible {
		t.Fatalf("expected reminder to bypass generic proactive restrictions")
	}
}

func TestScoreCandidate_PrefersEventFollowup(t *testing.T) {
	now := time.Date(2026, 4, 9, 21, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	session := SessionSnapshot{
		RelationshipScore: 80,
		CurrentMood:       "neutral",
	}
	profile := ProactiveProfile{
		ProactiveSuccessScore: 0.7,
		BestTimeWindows: []TimeWindow{
			{Start: "20:00", End: "22:00", Weekdays: []string{"thu"}},
		},
	}
	eventCandidate := TriggerCandidate{TriggerType: TriggerEventFollowup, TriggerRefID: "event:1"}
	reconnectCandidate := TriggerCandidate{TriggerType: TriggerReconnect, TriggerRefID: "reconnect:1"}

	eventScore := ScoreCandidate(session, profile, eventCandidate, []ConversationMessage{{Role: "user", CreatedAt: now.Add(-1 * time.Hour)}}, nil, now)
	reconnectScore := ScoreCandidate(session, profile, reconnectCandidate, []ConversationMessage{{Role: "user", CreatedAt: now.Add(-1 * time.Hour)}}, nil, now)

	if eventScore.Score <= reconnectScore.Score {
		t.Fatalf("expected event followup score to be higher, got event=%v reconnect=%v", eventScore.Score, reconnectScore.Score)
	}
}

func TestComposer_FallsBackWithoutLLM(t *testing.T) {
	cfg := DefaultConfig()
	composer := NewComposer(cfg, nil, nil, nil, nil)

	result, err := composer.Compose(nil, ComposeInput{
		Session:   SessionSnapshot{Mode: "spicy"},
		Candidate: TriggerCandidate{TriggerType: TriggerReconnect, TriggerRefID: "reconnect:1"},
		Strategy: Strategy{
			Type:      "reconnect",
			Intensity: IntensityLight,
			Tone:      ToneSpicy,
			Purpose:   PurposeCheckin,
			Length:    LengthShort,
		},
	})
	if err != nil {
		t.Fatalf("compose should not fail without llm: %v", err)
	}
	if result.Message == "" {
		t.Fatalf("expected fallback message")
	}
}
