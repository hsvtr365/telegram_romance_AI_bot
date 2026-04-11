package proactive

import (
	"context"
	"testing"
	"time"
)

func TestPickBestCandidate_PrefersHigherPriority(t *testing.T) {
	cfg := DefaultConfig()
	scheduler := &Scheduler{cfg: cfg, clock: fixedClock{now: testKSTTime(2026, 4, 10, 20, 30)}}
	now := scheduler.clock.Now()

	session := SessionSnapshot{
		SessionID:         1,
		UserID:            1,
		ProactiveOptIn:    true,
		TimezoneName:      DefaultTimezoneName,
		RelationshipScore: 80,
		CurrentMood:       "sad",
		LastUserMessageAt: now.Add(-2 * time.Hour),
	}
	profile := ProactiveProfile{
		MinGapHours:     20,
		MaxPerDay:       2,
		MaxPerWeek:      4,
		BestTimeWindows: []TimeWindow{{Weekdays: []string{"fri"}, Start: "19:00", End: "21:30"}},
		PreferredTypes:  []string{string(TriggerHabitPing)},
		TimezoneName:    DefaultTimezoneName,
	}
	recentMessages := []ConversationMessage{
		{Role: "user", CreatedAt: now.Add(-2 * time.Hour), Content: "오늘 좀 속상해"},
		{Role: "user", CreatedAt: now.Add(-26 * time.Hour), Content: "어제도 피곤했어"},
		{Role: "user", CreatedAt: now.Add(-48 * time.Hour), Content: "바빠"},
		{Role: "user", CreatedAt: now.Add(-72 * time.Hour), Content: "끝났어"},
	}

	habit := ScannedCandidate{
		Session:          session,
		Profile:          profile,
		RecentMessages:   recentMessages,
		RecentProactives: nil,
		Candidate: TriggerCandidate{
			TriggerType:  TriggerHabitPing,
			TriggerRefID: "habit:1:2026-04-10:19:00",
			Priority:     60,
			TriggeredAt:  now,
		},
	}
	event := ScannedCandidate{
		Session:          session,
		Profile:          profile,
		RecentMessages:   recentMessages,
		RecentProactives: nil,
		Candidate: TriggerCandidate{
			TriggerType:  TriggerEventFollowup,
			TriggerRefID: "schedule:exam:1",
			Priority:     100,
			TriggeredAt:  now.Add(-5 * time.Minute),
		},
	}

	best, ok := scheduler.pickBestCandidate([]ScannedCandidate{habit, event}, now)
	if !ok {
		t.Fatalf("expected a winner")
	}
	if best.Candidate.TriggerType != TriggerEventFollowup {
		t.Fatalf("expected event_followup to win, got %s", best.Candidate.TriggerType)
	}
}

func TestPickBestCandidate_RejectsLowScoreCandidates(t *testing.T) {
	cfg := DefaultConfig()
	scheduler := &Scheduler{cfg: cfg, clock: fixedClock{now: testKSTTime(2026, 4, 10, 20, 30)}}
	now := scheduler.clock.Now()

	session := SessionSnapshot{
		SessionID:         1,
		UserID:            1,
		ProactiveOptIn:    true,
		TimezoneName:      DefaultTimezoneName,
		RelationshipScore: 20,
		LastUserMessageAt: now.Add(-96 * time.Hour),
	}
	profile := ProactiveProfile{
		MinGapHours:  20,
		MaxPerDay:    2,
		MaxPerWeek:   4,
		TimezoneName: DefaultTimezoneName,
	}

	item := ScannedCandidate{
		Session:        session,
		Profile:        profile,
		RecentMessages: nil,
		Candidate: TriggerCandidate{
			TriggerType:  TriggerReconnect,
			TriggerRefID: "reconnect:1:2026-04-10",
			Priority:     40,
			TriggeredAt:  now,
		},
	}

	if _, ok := scheduler.pickBestCandidate([]ScannedCandidate{item}, now); ok {
		t.Fatalf("expected low-score reconnect candidate to be skipped")
	}
}

func TestHandleScannedCandidate_InjectsMemorySummaryIntoComposeInput(t *testing.T) {
	now := testKSTTime(2026, 4, 10, 20, 30)
	repo := &memorySummaryRepo{
		memorySummary: "[Memory Summary]\n- topic: 시험 준비 | 오늘은 컨디션이 안 좋음",
	}
	mainLLM := &stubLLM{reply: "선톡 메시지"}
	scheduler := &Scheduler{
		cfg:      DefaultConfig(),
		repo:     repo,
		composer: NewComposer(DefaultConfig(), mainLLM, nil, nil, nil),
		sender:   NewSender(repo, nil, nil),
		clock:    fixedClock{now: now},
	}

	item := ScannedCandidate{
		Session: SessionSnapshot{
			SessionID:         10,
			UserID:            20,
			TelegramChatID:    30,
			Mode:              "soft",
			ProactiveOptIn:    true,
			TimezoneName:      DefaultTimezoneName,
			RelationshipScore: 88,
			RecentTurnLimit:   14,
			LastUserMessageAt: now.Add(-72 * time.Hour),
		},
		Profile: ProactiveProfile{
			UserID:                20,
			MinGapHours:           20,
			MaxPerDay:             2,
			MaxPerWeek:            4,
			TimezoneName:          DefaultTimezoneName,
			ProactiveSuccessScore: 0.8,
		},
		RecentMessages: []ConversationMessage{
			{Role: "user", Content: "최근에 시험 준비 때문에 좀 바빠", CreatedAt: now.Add(-2 * time.Hour)},
		},
		Candidate: TriggerCandidate{
			TriggerType:  TriggerEventFollowup,
			TriggerRefID: "event:exam:1",
			Priority:     100,
			TriggeredAt:  now,
		},
		Event: &MemoryEvent{
			ID:        7,
			SessionID: 10,
			EventType: "exam",
			EventTime: now.Add(-10 * time.Minute),
		},
	}

	if err := scheduler.handleScannedCandidate(context.Background(), item); err != nil {
		t.Fatalf("handleScannedCandidate failed: %v", err)
	}
	if repo.memorySummaryCalls != 1 {
		t.Fatalf("expected memory summary lookup once, got %d", repo.memorySummaryCalls)
	}
	if len(mainLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(mainLLM.calls))
	}
	if got := mainLLM.calls[0][1].Content; !containsAll(got, []string{"[Memory Summary]", "시험 준비", "컨디션이 안 좋음"}) {
		t.Fatalf("expected memory summary to be injected into prompt, got %q", got)
	}
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time { return f.now }

func testKSTTime(year int, month time.Month, day int, hour int, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.FixedZone("KST", 9*60*60))
}

type memorySummaryRepo struct {
	memorySummary      string
	memorySummaryCalls int
}

func (r *memorySummaryRepo) ListSessionsForProactive(context.Context, time.Time) ([]SessionSnapshot, error) {
	return nil, nil
}

func (r *memorySummaryRepo) ListDueMemoryEvents(context.Context, time.Time, int) ([]MemoryEvent, error) {
	return nil, nil
}

func (r *memorySummaryRepo) ListRecentMessages(context.Context, int64, int) ([]ConversationMessage, error) {
	return nil, nil
}

func (r *memorySummaryRepo) ListRecentProactiveMessages(context.Context, int64, int) ([]ProactiveMessageRecord, error) {
	return nil, nil
}

func (r *memorySummaryRepo) GetMemorySummary(_ context.Context, _ int64, _ int) (string, error) {
	r.memorySummaryCalls++
	return r.memorySummary, nil
}

func (r *memorySummaryRepo) GetProactiveProfile(context.Context, int64) (ProactiveProfile, error) {
	return ProactiveProfile{}, nil
}

func (r *memorySummaryRepo) InsertProactiveMessage(context.Context, ProactiveMessageRecord) (int64, error) {
	return 1, nil
}

func (r *memorySummaryRepo) UpdateProactiveMessage(context.Context, ProactiveMessageRecord) error {
	return nil
}

func (r *memorySummaryRepo) SaveConversationTurn(context.Context, int64, string, string, string, int) error {
	return nil
}

func (r *memorySummaryRepo) MarkMemoryEventUsedForProactive(context.Context, int64) error {
	return nil
}

func (r *memorySummaryRepo) UpdateSessionProactiveState(context.Context, int64, SessionProactivePatch) error {
	return nil
}

func (r *memorySummaryRepo) UpdateProactiveProfile(context.Context, int64, ProactiveProfilePatch) error {
	return nil
}

var _ Repository = (*memorySummaryRepo)(nil)
