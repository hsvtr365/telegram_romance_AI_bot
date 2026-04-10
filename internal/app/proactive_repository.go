package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
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
		out = append(out, proactive.SessionSnapshot{
			SessionID:                   item.Session.ID,
			UserID:                      item.User.ID,
			TelegramChatID:              item.User.TelegramChatID,
			Mode:                        item.Session.Mode,
			LastUserMessageAt:           item.Session.LastUserMessageAt,
			LastBotMessageAt:            item.Session.LastBotMessageAt,
			LastProactiveAt:             item.Session.LastProactiveAt,
			LastUserReplyToProactiveAt:  item.Session.LastUserReplyToProactiveAt,
			ConsecutiveProactiveIgnored: item.Session.ConsecutiveProactiveIgnored,
			RelationshipScore:           item.Session.RelationshipScore,
			CurrentMood:                 item.Session.CurrentMood,
			ProactiveOptIn:              item.Session.ProactiveOptIn,
			QuietHours:                  parseQuietWindows(item.Session.QuietHoursJSON),
			TimezoneName:                proactive.DefaultTimezoneName,
			LastMessageAt:               item.Session.LastMessageAt,
			RecentTurnLimit:             item.Session.RecentTurnLimit,
			UpdatedAt:                   item.Session.UpdatedAt,
		})
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
		out = append(out, proactive.MemoryEvent{
			ID:               event.ID,
			SessionID:        event.SessionID,
			MessageID:        event.MessageID,
			EventType:        event.EventType,
			EventSubtype:     event.EventSubtype,
			EventValue:       parseMap(event.EventValue),
			EventTime:        event.EventTime,
			Priority:         event.Priority,
			UsedForProactive: event.UsedForProactive,
			CreatedAt:        event.CreatedAt,
			UpdatedAt:        event.UpdatedAt,
		})
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
		out = append(out, proactive.ConversationMessage{
			ID:        message.ID,
			SessionID: message.SessionID,
			Role:      message.Role,
			Content:   message.Content,
			Mode:      message.Mode,
			CreatedAt: message.CreatedAt,
		})
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
		out = append(out, proactive.ProactiveMessageRecord{
			ID:                message.ID,
			SessionID:         message.SessionID,
			UserID:            message.UserID,
			TriggerType:       proactive.TriggerType(message.TriggerType),
			TriggerRefID:      message.TriggerRefID,
			StrategyType:      message.StrategyType,
			ToneMode:          proactive.ToneMode(message.ToneMode),
			Intensity:         proactive.Intensity(message.Intensity),
			Purpose:           proactive.Purpose(message.Purpose),
			Score:             message.Score,
			MessageText:       message.MessageText,
			SeedKey:           message.SeedKey,
			TelegramMessageID: message.TelegramMessageID,
			SentAt:            message.SentAt,
			DeliveryStatus:    proactive.DeliveryStatus(message.DeliveryStatus),
			UserReplied:       message.UserReplied,
			ReplyDelaySec:     message.ReplyDelaySec,
			ReplySentiment:    message.ReplySentiment,
			ReplyLength:       message.ReplyLength,
			FollowupTurnCount: message.FollowupTurnCount,
			CreatedAt:         message.CreatedAt,
			UpdatedAt:         message.UpdatedAt,
		})
	}

	return out, nil
}

func (r *proactiveRepository) GetProactiveProfile(ctx context.Context, userID int64) (proactive.ProactiveProfile, error) {
	profile, err := r.store.GetProactiveProfileByUserID(ctx, userID)
	if err != nil {
		return proactive.ProactiveProfile{}, err
	}

	return proactive.ProactiveProfile{
		UserID:                profile.UserID,
		PreferredTypes:        parseStringSlice(profile.PreferredTypesJSON),
		DislikedTypes:         parseStringSlice(profile.DislikedTypesJSON),
		BestTimeWindows:       parseTimeWindows(profile.BestTimeWindowsJSON),
		MaxPerDay:             profile.MaxPerDay,
		MaxPerWeek:            profile.MaxPerWeek,
		MinGapHours:           profile.MinGapHours,
		AvgReplyDelaySec:      profile.AvgReplyDelaySec,
		ProactiveSuccessScore: profile.ProactiveSuccessScore,
		TimezoneName:          fallbackString(profile.TimezoneName, proactive.DefaultTimezoneName),
		WeightOverrides:       parseFloatMap(profile.WeightOverridesJSON),
	}, nil
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
		payload, err := json.Marshal(patch.QuietHours)
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
		current = zeroProfile(userID)
	}

	preferred := parseStringSlice(current.PreferredTypesJSON)
	if patch.PreferredTypes != nil {
		preferred = patch.PreferredTypes
	}
	disliked := parseStringSlice(current.DislikedTypesJSON)
	if patch.DislikedTypes != nil {
		disliked = patch.DislikedTypes
	}
	bestWindows := parseTimeWindows(current.BestTimeWindowsJSON)
	if patch.BestTimeWindows != nil {
		bestWindows = patch.BestTimeWindows
	}
	weightOverrides := parseFloatMap(current.WeightOverridesJSON)
	if patch.WeightOverrides != nil {
		weightOverrides = patch.WeightOverrides
	}

	maxPerDay := current.MaxPerDay
	if patch.MaxPerDay != nil {
		maxPerDay = *patch.MaxPerDay
	}
	maxPerWeek := current.MaxPerWeek
	if patch.MaxPerWeek != nil {
		maxPerWeek = *patch.MaxPerWeek
	}
	minGapHours := current.MinGapHours
	if patch.MinGapHours != nil {
		minGapHours = *patch.MinGapHours
	}
	avgReplyDelay := current.AvgReplyDelaySec
	if patch.AvgReplyDelaySec != nil {
		avgReplyDelay = *patch.AvgReplyDelaySec
	}
	successScore := current.ProactiveSuccessScore
	if patch.ProactiveSuccessScore != nil {
		successScore = *patch.ProactiveSuccessScore
	}
	timezoneName := fallbackString(current.TimezoneName, proactive.DefaultTimezoneName)
	if patch.TimezoneName != nil && *patch.TimezoneName != "" {
		timezoneName = *patch.TimezoneName
	}

	preferredJSON, err := json.Marshal(preferred)
	if err != nil {
		return err
	}
	dislikedJSON, err := json.Marshal(disliked)
	if err != nil {
		return err
	}
	bestWindowsJSON, err := json.Marshal(bestWindows)
	if err != nil {
		return err
	}
	weightOverridesJSON, err := json.Marshal(weightOverrides)
	if err != nil {
		return err
	}

	_, err = r.store.UpsertProactiveProfile(ctx, pgstore.UpsertProactiveProfileParams{
		UserID:                userID,
		PreferredTypesJSON:    preferredJSON,
		DislikedTypesJSON:     dislikedJSON,
		BestTimeWindowsJSON:   bestWindowsJSON,
		MaxPerDay:             maxPerDay,
		MaxPerWeek:            maxPerWeek,
		MinGapHours:           minGapHours,
		AvgReplyDelaySec:      avgReplyDelay,
		ProactiveSuccessScore: successScore,
		TimezoneName:          timezoneName,
		WeightOverridesJSON:   weightOverridesJSON,
	})
	return err
}

func zeroProfile(userID int64) model.ProactiveProfile {
	return model.ProactiveProfile{
		UserID:                userID,
		MaxPerDay:             proactive.DefaultMaxPerDay,
		MaxPerWeek:            proactive.DefaultMaxPerWeek,
		MinGapHours:           proactive.DefaultMinGapHours,
		TimezoneName:          proactive.DefaultTimezoneName,
		ProactiveSuccessScore: 0,
	}
}

func parseQuietWindows(raw []byte) []proactive.QuietWindow {
	if len(raw) == 0 {
		return nil
	}
	var windows []proactive.QuietWindow
	_ = json.Unmarshal(raw, &windows)
	return windows
}

func parseTimeWindows(raw []byte) []proactive.TimeWindow {
	if len(raw) == 0 {
		return nil
	}
	var windows []proactive.TimeWindow
	_ = json.Unmarshal(raw, &windows)
	return windows
}

func parseStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}

func parseFloatMap(raw []byte) map[string]float64 {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]float64
	_ = json.Unmarshal(raw, &values)
	return values
}

func parseMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	return values
}

func fallbackString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
