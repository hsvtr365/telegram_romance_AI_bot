package holiday

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type SyncStore interface {
	ReplaceSpecialDaysForMonthKind(ctx context.Context, year int, month time.Month, kindCode string, items []model.SpecialDay) error
	HasAnySpecialDays(ctx context.Context) (bool, error)
}

type Fetcher interface {
	FetchMonth(ctx context.Context, kind SpecialDayKind, year int, month time.Month) ([]model.SpecialDay, error)
}

type Syncer struct {
	store    SyncStore
	client   Fetcher
	interval time.Duration
	logger   *slog.Logger
	clock    clock
}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func NewSyncer(store SyncStore, client Fetcher, interval time.Duration, logger *slog.Logger) *Syncer {
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	return &Syncer{
		store:    store,
		client:   client,
		interval: interval,
		logger:   logger,
		clock:    realClock{},
	}
}

func (s *Syncer) Run(ctx context.Context) error {
	if s == nil || s.store == nil || s.client == nil {
		<-ctx.Done()
		return ctx.Err()
	}

	s.syncRange(ctx, s.clock.Now())

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.syncRange(ctx, s.clock.Now())
		}
	}
}

func (s *Syncer) syncRange(ctx context.Context, now time.Time) {
	hasAny, err := s.store.HasAnySpecialDays(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("holiday sync precheck failed", "error", err)
		}
		return
	}
	if hasAny {
		return
	}

	years := []int{now.Year(), now.Year() + 1}
	for _, year := range years {
		for month := time.January; month <= time.December; month++ {
			for _, kind := range SupportedKinds {
				if err := s.syncMonth(ctx, kind, year, month); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					if errors.Is(err, ErrUnauthorized) {
						if s.logger != nil {
							s.logger.Warn("holiday sync disabled due to unauthorized API key", "kind", kind.Endpoint, "year", year, "month", int(month))
						}
						return
					}
					if s.logger != nil {
						s.logger.Warn("holiday month sync failed", "kind", kind.Endpoint, "year", year, "month", int(month), "error", err)
					}
				}
			}
		}
	}
}

func (s *Syncer) syncMonth(ctx context.Context, kind SpecialDayKind, year int, month time.Month) error {
	items, err := s.client.FetchMonth(ctx, kind, year, month)
	if err != nil {
		return fmt.Errorf("fetch holiday month: %w", err)
	}
	if err := s.store.ReplaceSpecialDaysForMonthKind(ctx, year, month, kind.Code, items); err != nil {
		return fmt.Errorf("replace holiday month: %w", err)
	}
	return nil
}
