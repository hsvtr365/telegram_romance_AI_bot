package telegram

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/pkg/logx"
)

type UpdateHandler interface {
	HandleUpdate(ctx context.Context, update Update) error
}

type PollingConfig struct {
	TimeoutSec     int
	Limit          int
	AllowedUpdates []string
}

type Poller struct {
	cfg     PollingConfig
	client  *Client
	handler UpdateHandler
	logger  *slog.Logger
}

func NewPoller(cfg PollingConfig, client *Client, handler UpdateHandler, logger *slog.Logger) *Poller {
	return &Poller{
		cfg:     cfg,
		client:  client,
		handler: handler,
		logger:  logger,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	var offset int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := p.client.GetUpdates(ctx, offset, p.cfg.TimeoutSec, p.cfg.Limit, p.cfg.AllowedUpdates)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			p.logger.Error("텔레그램 폴링에 실패했습니다.", "원인", logx.KoreanError(err))
			if err := sleepWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
			continue
		}

		for _, update := range updates {
			go func(u Update) {
				if err := p.handler.HandleUpdate(ctx, u); err != nil {
					p.logger.Error("업데이트 처리에 실패했습니다.", "update_id", u.UpdateID, "원인", logx.KoreanError(err))
				}
			}(update)
			offset = update.UpdateID + 1
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
