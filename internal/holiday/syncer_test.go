package holiday

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type syncStoreStub struct {
	calls []string
}

func (s *syncStoreStub) ReplaceSpecialDaysForMonthKind(_ context.Context, year int, month time.Month, kindCode string, items []model.SpecialDay) error {
	s.calls = append(s.calls, fmt.Sprintf("%d-%02d:%s:%d", year, int(month), kindCode, len(items)))
	return nil
}

type fetcherStub struct {
	failFor map[string]bool
}

func (f *fetcherStub) FetchMonth(_ context.Context, kind SpecialDayKind, year int, month time.Month) ([]model.SpecialDay, error) {
	key := fmt.Sprintf("%d-%02d:%s", year, int(month), kind.Code)
	if f.failFor[key] {
		return nil, fmt.Errorf("boom")
	}
	return []model.SpecialDay{{
		Day:       time.Date(year, month, 1, 0, 0, 0, 0, time.UTC),
		Name:      "sample",
		KindCode:  kind.Code,
		KindLabel: kind.Label,
		FetchedAt: time.Now().UTC(),
	}}, nil
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time { return f.now }

func TestSyncerSyncRange_ContinuesOnMonthFailure(t *testing.T) {
	store := &syncStoreStub{}
	fetcher := &fetcherStub{
		failFor: map[string]bool{
			"2026-01:restde": true,
		},
	}

	syncer := NewSyncer(store, fetcher, 24*time.Hour, nil)
	syncer.clock = fixedClock{now: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)}

	syncer.syncRange(context.Background(), syncer.clock.Now())

	expectedCalls := 24*len(SupportedKinds) - 1
	if len(store.calls) != expectedCalls {
		t.Fatalf("expected %d successful replacements, got %d", expectedCalls, len(store.calls))
	}
}
