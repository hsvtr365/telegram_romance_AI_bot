package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
	"github.com/hsvtr365/telegram_romance_AI_bot/pkg/logx"
)

type PollingConfig struct {
	BotID          string
	TimeoutSec     int
	Limit          int
	AllowedUpdates []string
}

type Poller struct {
	cfg     PollingConfig
	client  *Client
	handler channelx.MessageHandler
	logger  *slog.Logger
}

func NewPoller(cfg PollingConfig, client *Client, handler channelx.MessageHandler, logger *slog.Logger) *Poller {
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
			message, ok := inboundFromUpdate(p.cfg.BotID, update)
			if !ok {
				offset = update.UpdateID + 1
				continue
			}
			go func(u Update, msg channelx.InboundMessage) {
				if err := p.handler.HandleMessage(ctx, msg); err != nil {
					p.logger.Error("업데이트 처리에 실패했습니다.", "update_id", u.UpdateID, "원인", logx.KoreanError(err))
				}
			}(update, message)
			offset = update.UpdateID + 1
		}
	}
}

func inboundFromUpdate(botID string, update Update) (channelx.InboundMessage, bool) {
	if update.Message == nil || update.Message.From == nil {
		return channelx.InboundMessage{}, false
	}
	if update.Message.Chat.Type != "private" {
		return channelx.InboundMessage{}, false
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return channelx.InboundMessage{}, false
	}

	sentAt := time.Now().UTC()
	if update.Message.Date > 0 {
		sentAt = time.Unix(update.Message.Date, 0).UTC()
	}

	return channelx.InboundMessage{
		BotID:             botID,
		Channel:           channelx.Telegram,
		ExternalUserID:    strconv.FormatInt(update.Message.From.ID, 10),
		ExternalChatID:    strconv.FormatInt(update.Message.Chat.ID, 10),
		ExternalMessageID: strconv.FormatInt(update.Message.MessageID, 10),
		ExternalUpdateID:  strconv.FormatInt(update.UpdateID, 10),
		ChatType:          channelx.ChatTypePrivate,
		Text:              text,
		Username:          update.Message.From.Username,
		FirstName:         update.Message.From.FirstName,
		SentAt:            sentAt,
	}, true
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
