package proactive

import (
	"context"
	"log/slog"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
)

type Sender struct {
	repo    Repository
	bot     Messenger
	holiday holiday.ContextResolver
	logger  *slog.Logger
}

func NewSender(repo Repository, bot Messenger, logger *slog.Logger) *Sender {
	return &Sender{
		repo:   repo,
		bot:    bot,
		logger: logger,
	}
}

func (s *Sender) SetHolidayResolver(resolver holiday.ContextResolver) {
	if s == nil {
		return
	}
	s.holiday = resolver
}

func (s *Sender) Send(ctx context.Context, session SessionSnapshot, decision Decision, composeResult ComposeResult) (ProactiveMessageRecord, error) {
	candidate := decision.Candidate
	strategy := decision.Strategy
	record := ProactiveMessageRecord{
		SessionID:      session.SessionID,
		UserID:         session.UserID,
		TriggerType:    candidate.TriggerType,
		TriggerRefID:   candidate.TriggerRefID,
		StrategyType:   strategy.Type,
		ToneMode:       strategy.Tone,
		Intensity:      strategy.Intensity,
		Purpose:        strategy.Purpose,
		Score:          decision.Score.Score,
		MessageText:    composeResult.Message,
		SeedKey:        composeResult.Seed.Key,
		DeliveryStatus: DeliveryQueued,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if s.repo != nil {
		id, err := s.repo.InsertProactiveMessage(ctx, record)
		if err != nil {
			return record, err
		}
		record.ID = id
	}

	if s.bot != nil {
		if err := s.bot.SendChatAction(ctx, session.TelegramChatID, "typing"); err != nil && s.logger != nil {
			s.logger.Warn("proactive send chat action failed", "session_id", session.SessionID, "error", err)
		}
		if err := s.sendText(ctx, session.TelegramChatID, composeResult.Message); err != nil {
			record.DeliveryStatus = DeliveryFailed
			record.UpdatedAt = time.Now()
			if s.repo != nil {
				_ = s.repo.UpdateProactiveMessage(ctx, record)
			}
			return record, err
		}
	}

	record.DeliveryStatus = DeliverySent
	record.SentAt = time.Now()
	record.UpdatedAt = time.Now()

	if s.repo != nil {
		if err := s.repo.UpdateProactiveMessage(ctx, record); err != nil && s.logger != nil {
			s.logger.Warn("failed to update proactive message after send", "session_id", session.SessionID, "error", err)
		}
		if err := s.repo.SaveConversationTurn(ctx, session.SessionID, "assistant", composeResult.Message, session.Mode, session.RecentTurnLimit); err != nil && s.logger != nil {
			s.logger.Warn("failed to persist proactive turn in conversation log", "session_id", session.SessionID, "error", err)
		}
		if err := s.repo.UpdateSessionProactiveState(ctx, session.SessionID, SessionProactivePatch{
			LastProactiveAt: &record.SentAt,
		}); err != nil && s.logger != nil {
			s.logger.Warn("failed to update proactive session state", "session_id", session.SessionID, "error", err)
		}
	}

	if composeResult.HolidayPromptUsed && composeResult.HolidayTopic != nil && s.holiday != nil {
		if err := s.holiday.MarkUsed(ctx, session.SessionID, holiday.SourceProactive, *composeResult.HolidayTopic); err != nil && s.logger != nil {
			s.logger.Warn("failed to mark proactive holiday topic used", "session_id", session.SessionID, "topic_key", composeResult.HolidayTopic.TopicKey, "error", err)
		}
	}

	return record, nil
}

func (s *Sender) sendText(ctx context.Context, chatID int64, text string) error {
	parts := chat.SplitReplyForTelegram(text)
	if len(parts) == 0 {
		parts = []string{text}
	}

	for _, part := range parts {
		if err := s.bot.SendMessage(ctx, chatID, part); err != nil {
			return err
		}
	}

	return nil
}
