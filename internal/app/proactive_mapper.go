package app

import (
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

func mapProactiveSession(item model.ProactiveSession) proactive.SessionSnapshot {
	return proactive.SessionSnapshot{
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
	}
}

func mapMemoryEvent(event model.MemoryEvent) proactive.MemoryEvent {
	return proactive.MemoryEvent{
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
	}
}

func mapConversationMessage(message model.Message) proactive.ConversationMessage {
	return proactive.ConversationMessage{
		ID:        message.ID,
		SessionID: message.SessionID,
		Role:      message.Role,
		Content:   message.Content,
		Mode:      message.Mode,
		CreatedAt: message.CreatedAt,
	}
}

func mapProactiveMessageRecord(message model.ProactiveMessage) proactive.ProactiveMessageRecord {
	return proactive.ProactiveMessageRecord{
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
	}
}

func mapProactiveProfile(profile model.ProactiveProfile) proactive.ProactiveProfile {
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
	}
}

func defaultProactiveProfile(userID int64) model.ProactiveProfile {
	return model.ProactiveProfile{
		UserID:                userID,
		MaxPerDay:             proactive.DefaultMaxPerDay,
		MaxPerWeek:            proactive.DefaultMaxPerWeek,
		MinGapHours:           proactive.DefaultMinGapHours,
		TimezoneName:          proactive.DefaultTimezoneName,
		ProactiveSuccessScore: 0,
	}
}

func mergeProactiveProfilePatch(userID int64, current model.ProactiveProfile, patch proactive.ProactiveProfilePatch) (pgstore.UpsertProactiveProfileParams, error) {
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
	if patch.TimezoneName != nil && fallbackString(*patch.TimezoneName, "") != "" {
		timezoneName = fallbackString(*patch.TimezoneName, timezoneName)
	}

	preferredJSON, err := marshalJSON(preferred)
	if err != nil {
		return pgstore.UpsertProactiveProfileParams{}, err
	}

	dislikedJSON, err := marshalJSON(disliked)
	if err != nil {
		return pgstore.UpsertProactiveProfileParams{}, err
	}

	bestWindowsJSON, err := marshalJSON(bestWindows)
	if err != nil {
		return pgstore.UpsertProactiveProfileParams{}, err
	}

	weightOverridesJSON, err := marshalJSON(weightOverrides)
	if err != nil {
		return pgstore.UpsertProactiveProfileParams{}, err
	}

	return pgstore.UpsertProactiveProfileParams{
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
	}, nil
}
