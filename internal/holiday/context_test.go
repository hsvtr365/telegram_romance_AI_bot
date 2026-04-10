package holiday

import (
	"context"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type resolverStoreStub struct {
	days  []model.SpecialDay
	used  map[string]bool
	marks []model.SessionPromptTopic
}

func (s *resolverStoreStub) ListSpecialDays(_ context.Context, from time.Time, to time.Time) ([]model.SpecialDay, error) {
	return s.days, nil
}

func (s *resolverStoreStub) HasSessionPromptTopic(_ context.Context, sessionID int64, topicKey string) (bool, error) {
	return s.used[topicKey], nil
}

func (s *resolverStoreStub) MarkSessionPromptTopicUsed(_ context.Context, topic model.SessionPromptTopic) error {
	s.marks = append(s.marks, topic)
	if s.used == nil {
		s.used = make(map[string]bool)
	}
	s.used[topic.TopicKey] = true
	return nil
}

func TestResolver_PrefersTodayHolidayAndSkipsUsedTopic(t *testing.T) {
	store := &resolverStoreStub{
		days: []model.SpecialDay{
			{Day: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), Name: "한글날", IsHoliday: true, Seq: 1},
			{Day: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), Name: "추석연휴", IsHoliday: true, IsMajorHoliday: true, MajorHolidayGroup: "chuseok", Seq: 1},
		},
		used: map[string]bool{
			"today_holiday:2026-10-09:한글날": true,
		},
	}

	resolver := NewResolver(store, ResolverConfig{
		LookaheadDays:   7,
		TodayPercent:    100,
		UpcomingPercent: 100,
	}, nil)

	selection, err := resolver.Resolve(context.Background(), ResolveInput{
		SessionID: 10,
		Now:       time.Date(2026, 10, 9, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		GateSeed:  "42",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if selection == nil {
		t.Fatalf("expected upcoming holiday selection")
	}
	if selection.TopicType != "upcoming_major_holiday" {
		t.Fatalf("expected upcoming major holiday selection, got %+v", selection)
	}
	if selection.TopicKey != "upcoming_major_holiday:2026-10-12:chuseok" {
		t.Fatalf("unexpected topic key: %q", selection.TopicKey)
	}
}

func TestResolver_MarkUsedPreventsReuse(t *testing.T) {
	store := &resolverStoreStub{
		days: []model.SpecialDay{
			{Day: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Name: "신정", IsHoliday: true, Seq: 1},
		},
		used: map[string]bool{},
	}

	resolver := NewResolver(store, ResolverConfig{
		LookaheadDays:   7,
		TodayPercent:    100,
		UpcomingPercent: 100,
	}, nil)

	selection, err := resolver.Resolve(context.Background(), ResolveInput{
		SessionID: 22,
		Now:       time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		GateSeed:  "1",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if selection == nil {
		t.Fatalf("expected selection")
	}

	if err := resolver.MarkUsed(context.Background(), 22, SourceChat, *selection); err != nil {
		t.Fatalf("MarkUsed returned error: %v", err)
	}

	selection, err = resolver.Resolve(context.Background(), ResolveInput{
		SessionID: 22,
		Now:       time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		GateSeed:  "1",
	})
	if err != nil {
		t.Fatalf("second Resolve returned error: %v", err)
	}
	if selection != nil {
		t.Fatalf("expected used topic to be skipped, got %+v", selection)
	}
	if len(store.marks) != 1 {
		t.Fatalf("expected one usage mark, got %d", len(store.marks))
	}
}
