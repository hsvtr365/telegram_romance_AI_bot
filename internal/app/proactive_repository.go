package app

import (
	"context"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type proactiveRepository struct {
	store *store.Manager
}

func newProactiveRepository(conversationStore *store.Manager) *proactiveRepository {
	return &proactiveRepository{store: conversationStore}
}

func (r *proactiveRepository) ListSessionsForProactive(ctx context.Context, _ time.Time) ([]proactive.SessionSnapshot, error) {
	items, err := r.store.ListActiveProactiveSessions(ctx, 200)
	if err != nil {
		return nil, err
	}

	out := make([]proactive.SessionSnapshot, 0, len(items))
	for _, item := range items {
		out = append(out, mapProactiveSession(item))
	}

	return out, nil
}

func (r *proactiveRepository) ListDueMemoryEvents(ctx context.Context, now time.Time, limit int) ([]proactive.MemoryEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	events, err := r.store.ListDueMemoryEvents(ctx, now.Add(-6*time.Hour), now.Add(6*time.Hour), limit)
	if err != nil {
		return nil, err
	}

	out := make([]proactive.MemoryEvent, 0, len(events))
	for _, event := range events {
		out = append(out, mapMemoryEvent(event))
	}

	return out, nil
}

func (r *proactiveRepository) ListRecentMessages(ctx context.Context, sessionID int64, limit int) ([]proactive.ConversationMessage, error) {
	messages, err := r.store.ListConversationMessages(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]proactive.ConversationMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, mapConversationMessage(message))
	}

	return out, nil
}

func (r *proactiveRepository) ListRecentProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]proactive.ProactiveMessageRecord, error) {
	messages, err := r.store.ListRecentProactiveMessages(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]proactive.ProactiveMessageRecord, 0, len(messages))
	for _, message := range messages {
		out = append(out, mapProactiveMessageRecord(message))
	}

	return out, nil
}

func (r *proactiveRepository) GetMemorySummary(ctx context.Context, sessionID int64, recentTurnLimit int) (string, error) {
	if sessionID == 0 {
		return "", nil
	}
	if recentTurnLimit <= 0 {
		recentTurnLimit = proactive.DefaultRecentConversationLimit
	}

	snapshot, err := r.store.BuildMemorySlotSnapshot(ctx, sessionID, recentTurnLimit)
	if err != nil {
		return "", err
	}

	return chat.BuildMemorySummaryFromStateMachine(snapshot.TopicSlots, snapshot.ConversationStateMachine), nil
}
 
func (r *proactiveRepository) ListSessionCustomSlots(ctx context.Context, sessionID int64) ([]model.CustomSlot, error) {
	return r.store.ListSessionCustomSlots(ctx, sessionID)
}

func (r *proactiveRepository) GetProactiveProfile(ctx context.Context, userID int64) (proactive.ProactiveProfile, error) {
	profile, err := r.store.GetProactiveProfileByUserID(ctx, userID)
	if err != nil {
		return proactive.ProactiveProfile{}, err
	}

	return mapProactiveProfile(profile), nil
}

func (r *proactiveRepository) InsertProactiveMessage(ctx context.Context, record proactive.ProactiveMessageRecord) (int64, error) {
	inserted, err := r.store.InsertProactiveMessage(ctx, pgstore.InsertProactiveMessageParams{
		SessionID:         record.SessionID,
		UserID:            record.UserID,
		TriggerType:       string(record.TriggerType),
		TriggerRefID:      record.TriggerRefID,
		StrategyType:      record.StrategyType,
		ToneMode:          string(record.ToneMode),
		Intensity:         string(record.Intensity),
		Purpose:           string(record.Purpose),
		Score:             record.Score,
		MessageText:       record.MessageText,
		SeedKey:           record.SeedKey,
		TelegramMessageID: record.TelegramMessageID,
		SentAt:            record.SentAt,
		DeliveryStatus:    string(record.DeliveryStatus),
		UserReplied:       record.UserReplied,
		ReplyDelaySec:     record.ReplyDelaySec,
		ReplySentiment:    record.ReplySentiment,
		ReplyLength:       record.ReplyLength,
		FollowupTurnCount: record.FollowupTurnCount,
	})
	if err != nil {
		return 0, err
	}
	return inserted.ID, nil
}

func (r *proactiveRepository) UpdateProactiveMessage(ctx context.Context, record proactive.ProactiveMessageRecord) error {
	if record.UserReplied {
		return r.store.MarkProactiveMessageReplied(ctx, record.ID, record.ReplyDelaySec, record.ReplySentiment, record.ReplyLength, record.FollowupTurnCount)
	}
	return r.store.UpdateProactiveMessageDelivery(ctx, record.ID, record.TelegramMessageID, record.SentAt, string(record.DeliveryStatus))
}

func (r *proactiveRepository) SaveConversationTurn(ctx context.Context, sessionID int64, role string, content string, mode string, recentTurnLimit int) error {
	return r.store.SaveTurn(ctx, sessionID, role, content, 0, 0, fallbackString(mode, chat.DefaultSessionMode), recentTurnLimit)
}

func (r *proactiveRepository) MarkMemoryEventUsedForProactive(ctx context.Context, eventID int64) error {
	return r.store.MarkMemoryEventUsedForProactive(ctx, eventID)
}

func (r *proactiveRepository) UpdateSessionProactiveState(ctx context.Context, sessionID int64, patch proactive.SessionProactivePatch) error {
	if patch.LastUserMessageAt != nil {
		if err := r.store.TouchSessionMessage(ctx, sessionID, "user", *patch.LastUserMessageAt); err != nil {
			return err
		}
	}
	if patch.LastBotMessageAt != nil {
		if err := r.store.TouchSessionMessage(ctx, sessionID, "assistant", *patch.LastBotMessageAt); err != nil {
			return err
		}
	}
	if patch.LastProactiveAt != nil {
		if err := r.store.MarkSessionProactiveSent(ctx, sessionID, *patch.LastProactiveAt); err != nil {
			return err
		}
	}
	if patch.LastUserReplyToProactiveAt != nil {
		if err := r.store.MarkSessionProactiveReplied(ctx, sessionID, *patch.LastUserReplyToProactiveAt); err != nil {
			return err
		}
	}
	if patch.ConsecutiveProactiveIgnored != nil {
		if err := r.store.SetSessionConsecutiveProactiveIgnored(ctx, sessionID, *patch.ConsecutiveProactiveIgnored); err != nil {
			return err
		}
	}
	if patch.RelationshipScore != nil {
		if err := r.store.SetSessionRelationshipScore(ctx, sessionID, *patch.RelationshipScore); err != nil {
			return err
		}
	}
	if patch.CurrentMood != nil {
		if err := r.store.SetSessionCurrentMood(ctx, sessionID, *patch.CurrentMood); err != nil {
			return err
		}
	}
	if patch.ProactiveOptIn != nil {
		if err := r.store.SetSessionProactiveOptIn(ctx, sessionID, *patch.ProactiveOptIn); err != nil {
			return err
		}
	}
	if patch.QuietHours != nil {
		payload, err := marshalJSON(patch.QuietHours)
		if err != nil {
			return err
		}
		if err := r.store.SetSessionQuietHours(ctx, sessionID, payload); err != nil {
			return err
		}
	}
	return nil
}

func (r *proactiveRepository) UpdateProactiveProfile(ctx context.Context, userID int64, patch proactive.ProactiveProfilePatch) error {
	current, err := r.store.GetProactiveProfileByUserID(ctx, userID)
	if err != nil {
		current = defaultProactiveProfile(userID)
	}

	params, err := mergeProactiveProfilePatch(userID, current, patch)
	if err != nil {
		return err
	}

	_, err = r.store.UpsertProactiveProfile(ctx, params)
	return err
}
