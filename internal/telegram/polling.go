package telegram

import (
	"context"
	"log/slog"
	"time"
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
			p.logger.Error("telegram polling failed", "error", err)
			if err := sleepWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
			continue
		}

		for _, update := range updates {
			if err := p.handler.HandleUpdate(ctx, update); err != nil {
				p.logger.Error("update handling failed", "update_id", update.UpdateID, "error", err)
			}
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
