package proactive

import (
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

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time { return f.now }

func testKSTTime(year int, month time.Month, day int, hour int, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.FixedZone("KST", 9*60*60))
}

