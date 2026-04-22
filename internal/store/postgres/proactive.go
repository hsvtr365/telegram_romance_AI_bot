package postgres

import (
	"context"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type UpsertProactiveProfileParams struct {
	UserID                int64
	PreferredTypesJSON    []byte
	DislikedTypesJSON     []byte
	BestTimeWindowsJSON   []byte
	MaxPerDay             int
	MaxPerWeek            int
	MinGapHours           int
	AvgReplyDelaySec      int
	ProactiveSuccessScore float64
	TimezoneName          string
	WeightOverridesJSON   []byte
}

type InsertMemoryEventParams struct {
	SessionID        int64
	MessageID        int64
	EventType        string
	EventSubtype     string
	EventValue       []byte
	EventTime        time.Time
	Priority         int
	UsedForProactive bool
}

type InsertProactiveMessageParams struct {
	SessionID         int64
	UserID            int64
	TriggerType       string
	TriggerRefID      string
	StrategyType      string
	ToneMode          string
	Intensity         string
	Purpose           string
	Score             float64
	MessageText       string
	SeedKey           string
	Channel           string
	ExternalMessageID string
	TelegramMessageID int64
	SentAt            time.Time
	DeliveryStatus    string
	UserReplied       bool
	ReplyDelaySec     int
	ReplySentiment    string
	ReplyLength       int
	FollowupTurnCount int
}

func (s *Store) UpsertProactiveProfile(ctx context.Context, params UpsertProactiveProfileParams) (model.ProactiveProfile, error) {
	const query = `
INSERT INTO tg_proactive_profiles (
    user_id,
    preferred_types_json,
    disliked_types_json,
    best_time_windows_json,
    max_per_day,
    max_per_week,
    min_gap_hours,
    avg_reply_delay_sec,
    proactive_success_score,
    timezone_name,
    weight_overrides_json
) VALUES ($1, COALESCE($2::jsonb, '[]'::jsonb), COALESCE($3::jsonb, '[]'::jsonb), COALESCE($4::jsonb, '[]'::jsonb), $5, $6, $7, NULLIF($8, 0), $9, COALESCE(NULLIF($10, ''), 'Asia/Seoul'), COALESCE($11::jsonb, '{}'::jsonb))
ON CONFLICT (user_id) DO UPDATE
SET
    preferred_types_json = EXCLUDED.preferred_types_json,
    disliked_types_json = EXCLUDED.disliked_types_json,
    best_time_windows_json = EXCLUDED.best_time_windows_json,
    max_per_day = EXCLUDED.max_per_day,
    max_per_week = EXCLUDED.max_per_week,
    min_gap_hours = EXCLUDED.min_gap_hours,
    avg_reply_delay_sec = EXCLUDED.avg_reply_delay_sec,
    proactive_success_score = EXCLUDED.proactive_success_score,
    timezone_name = EXCLUDED.timezone_name,
    weight_overrides_json = EXCLUDED.weight_overrides_json,
    updated_at = NOW()
RETURNING
    id,
    user_id,
    preferred_types_json,
    disliked_types_json,
    best_time_windows_json,
    max_per_day,
    max_per_week,
    min_gap_hours,
    COALESCE(avg_reply_delay_sec, 0),
    proactive_success_score,
    timezone_name,
    weight_overrides_json,
    created_at,
    updated_at
`

	var profile model.ProactiveProfile
	err := s.pool.QueryRow(
		ctx,
		query,
		params.UserID,
		params.PreferredTypesJSON,
		params.DislikedTypesJSON,
		params.BestTimeWindowsJSON,
		params.MaxPerDay,
		params.MaxPerWeek,
		params.MinGapHours,
		params.AvgReplyDelaySec,
		params.ProactiveSuccessScore,
		params.TimezoneName,
		params.WeightOverridesJSON,
	).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.PreferredTypesJSON,
		&profile.DislikedTypesJSON,
		&profile.BestTimeWindowsJSON,
		&profile.MaxPerDay,
		&profile.MaxPerWeek,
		&profile.MinGapHours,
		&profile.AvgReplyDelaySec,
		&profile.ProactiveSuccessScore,
		&profile.TimezoneName,
		&profile.WeightOverridesJSON,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)

	return profile, err
}

func (s *Store) GetProactiveProfileByUserID(ctx context.Context, userID int64) (model.ProactiveProfile, error) {
	const query = `
SELECT
    id,
    user_id,
    preferred_types_json,
    disliked_types_json,
    best_time_windows_json,
    max_per_day,
    max_per_week,
    min_gap_hours,
    COALESCE(avg_reply_delay_sec, 0),
    proactive_success_score,
    timezone_name,
    weight_overrides_json,
    created_at,
    updated_at
FROM tg_proactive_profiles
WHERE user_id = $1
`

	var profile model.ProactiveProfile
	err := s.pool.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.PreferredTypesJSON,
		&profile.DislikedTypesJSON,
		&profile.BestTimeWindowsJSON,
		&profile.MaxPerDay,
		&profile.MaxPerWeek,
		&profile.MinGapHours,
		&profile.AvgReplyDelaySec,
		&profile.ProactiveSuccessScore,
		&profile.TimezoneName,
		&profile.WeightOverridesJSON,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)

	return profile, err
}

func (s *Store) InsertMemoryEvent(ctx context.Context, params InsertMemoryEventParams) (model.MemoryEvent, error) {
	const query = `
INSERT INTO tg_memory_events (
    session_id,
    message_id,
    event_type,
    event_subtype,
    event_value,
    event_time,
    priority,
    used_for_proactive
) VALUES ($1, NULLIF($2, 0), $3, $4, COALESCE($5::jsonb, '{}'::jsonb), $6, $7, $8)
ON CONFLICT (session_id, event_type, event_subtype, event_time) DO UPDATE
SET
    message_id = COALESCE(NULLIF(EXCLUDED.message_id, 0), tg_memory_events.message_id),
    event_value = EXCLUDED.event_value,
    priority = EXCLUDED.priority,
    used_for_proactive = EXCLUDED.used_for_proactive,
    updated_at = NOW()
RETURNING
    id,
    session_id,
    COALESCE(message_id, 0),
    event_type,
    event_subtype,
    event_value,
    event_time,
    priority,
    used_for_proactive,
    created_at,
    updated_at
`

	var event model.MemoryEvent
	err := s.pool.QueryRow(
		ctx,
		query,
		params.SessionID,
		params.MessageID,
		params.EventType,
		params.EventSubtype,
		params.EventValue,
		params.EventTime,
		params.Priority,
		params.UsedForProactive,
	).Scan(
		&event.ID,
		&event.SessionID,
		&event.MessageID,
		&event.EventType,
		&event.EventSubtype,
		&event.EventValue,
		&event.EventTime,
		&event.Priority,
		&event.UsedForProactive,
		&event.CreatedAt,
		&event.UpdatedAt,
	)

	return event, err
}

func (s *Store) ListRecentMemoryEvents(ctx context.Context, sessionID int64, limit int) ([]model.MemoryEvent, error) {
	const query = `
SELECT
    id,
    session_id,
    COALESCE(message_id, 0),
    event_type,
    event_subtype,
    event_value,
    event_time,
    priority,
    used_for_proactive,
    created_at,
    updated_at
FROM tg_memory_events
WHERE session_id = $1
ORDER BY priority DESC, event_time DESC, id DESC
LIMIT $2
`

	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]model.MemoryEvent, 0, limit)
	for rows.Next() {
		var event model.MemoryEvent
		if err := rows.Scan(
			&event.ID,
			&event.SessionID,
			&event.MessageID,
			&event.EventType,
			&event.EventSubtype,
			&event.EventValue,
			&event.EventTime,
			&event.Priority,
			&event.UsedForProactive,
			&event.CreatedAt,
			&event.UpdatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s *Store) MarkMemoryEventUsedForProactive(ctx context.Context, eventID int64) error {
	const query = `
UPDATE tg_memory_events
SET
    used_for_proactive = TRUE,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, eventID)
	return err
}

func (s *Store) InsertProactiveMessage(ctx context.Context, params InsertProactiveMessageParams) (model.ProactiveMessage, error) {
	const query = `
INSERT INTO tg_proactive_messages (
    session_id,
    user_id,
    trigger_type,
    trigger_ref_id,
    strategy_type,
    tone_mode,
    intensity,
    purpose,
    score,
    message_text,
    seed_key,
    channel,
    external_message_id,
    telegram_message_id,
    sent_at,
    delivery_status,
    user_replied,
    reply_delay_sec,
    reply_sentiment,
    reply_length,
    followup_turn_count
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, NULLIF($14, 0), NULLIF($15, TIMESTAMPTZ '0001-01-01 00:00:00+00'), COALESCE(NULLIF($16, ''), 'queued'), $17, NULLIF($18, 0), NULLIF($19, ''), NULLIF($20, 0), NULLIF($21, 0)
)
RETURNING
    id,
    session_id,
    user_id,
    trigger_type,
    trigger_ref_id,
    strategy_type,
    tone_mode,
    intensity,
    purpose,
    score,
    message_text,
    COALESCE(seed_key, ''),
    channel,
    COALESCE(external_message_id, ''),
    COALESCE(telegram_message_id, 0),
    COALESCE(sent_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    delivery_status,
    user_replied,
    COALESCE(reply_delay_sec, 0),
    COALESCE(reply_sentiment, ''),
    COALESCE(reply_length, 0),
    COALESCE(followup_turn_count, 0),
    created_at,
    updated_at
`

	var message model.ProactiveMessage
	err := s.pool.QueryRow(
		ctx,
		query,
		params.SessionID,
		params.UserID,
		params.TriggerType,
		params.TriggerRefID,
		params.StrategyType,
		params.ToneMode,
		params.Intensity,
		params.Purpose,
		params.Score,
		params.MessageText,
		params.SeedKey,
		normalizedChannel(params.Channel),
		params.ExternalMessageID,
		params.TelegramMessageID,
		params.SentAt,
		params.DeliveryStatus,
		params.UserReplied,
		params.ReplyDelaySec,
		params.ReplySentiment,
		params.ReplyLength,
		params.FollowupTurnCount,
	).Scan(
		&message.ID,
		&message.SessionID,
		&message.UserID,
		&message.TriggerType,
		&message.TriggerRefID,
		&message.StrategyType,
		&message.ToneMode,
		&message.Intensity,
		&message.Purpose,
		&message.Score,
		&message.MessageText,
		&message.SeedKey,
		&message.Channel,
		&message.ExternalMessageID,
		&message.TelegramMessageID,
		&message.SentAt,
		&message.DeliveryStatus,
		&message.UserReplied,
		&message.ReplyDelaySec,
		&message.ReplySentiment,
		&message.ReplyLength,
		&message.FollowupTurnCount,
		&message.CreatedAt,
		&message.UpdatedAt,
	)

	return message, err
}

func (s *Store) UpdateProactiveMessageDelivery(ctx context.Context, messageID int64, channelName string, externalMessageID string, telegramMessageID int64, sentAt time.Time, status string) error {
	const query = `
UPDATE tg_proactive_messages
SET
    channel = $2,
    external_message_id = $3,
    telegram_message_id = NULLIF($4, 0),
    sent_at = NULLIF($5, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    delivery_status = $6,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, messageID, normalizedChannel(channelName), externalMessageID, telegramMessageID, sentAt, status)
	return err
}

func (s *Store) MarkProactiveMessageReplied(ctx context.Context, messageID int64, replyDelaySec int, replySentiment string, replyLength int, followupTurnCount int) error {
	const query = `
UPDATE tg_proactive_messages
SET
    user_replied = TRUE,
    reply_delay_sec = NULLIF($2, 0),
    reply_sentiment = NULLIF($3, ''),
    reply_length = NULLIF($4, 0),
    followup_turn_count = NULLIF($5, 0),
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, messageID, replyDelaySec, replySentiment, replyLength, followupTurnCount)
	return err
}

func (s *Store) ListPendingProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]model.ProactiveMessage, error) {
	const query = `
SELECT
    id,
    session_id,
    user_id,
    trigger_type,
    trigger_ref_id,
    strategy_type,
    tone_mode,
    intensity,
    purpose,
    score,
    message_text,
    COALESCE(seed_key, ''),
    channel,
    COALESCE(external_message_id, ''),
    COALESCE(telegram_message_id, 0),
    COALESCE(sent_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    delivery_status,
    user_replied,
    COALESCE(reply_delay_sec, 0),
    COALESCE(reply_sentiment, ''),
    COALESCE(reply_length, 0),
    COALESCE(followup_turn_count, 0),
    created_at,
    updated_at
FROM tg_proactive_messages
WHERE session_id = $1
  AND user_replied = FALSE
ORDER BY created_at DESC
LIMIT $2
`

	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]model.ProactiveMessage, 0, limit)
	for rows.Next() {
		var message model.ProactiveMessage
		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.UserID,
			&message.TriggerType,
			&message.TriggerRefID,
			&message.StrategyType,
			&message.ToneMode,
			&message.Intensity,
			&message.Purpose,
			&message.Score,
			&message.MessageText,
			&message.SeedKey,
			&message.Channel,
			&message.ExternalMessageID,
			&message.TelegramMessageID,
			&message.SentAt,
			&message.DeliveryStatus,
			&message.UserReplied,
			&message.ReplyDelaySec,
			&message.ReplySentiment,
			&message.ReplyLength,
			&message.FollowupTurnCount,
			&message.CreatedAt,
			&message.UpdatedAt,
		); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (s *Store) ListRecentProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]model.ProactiveMessage, error) {
	const query = `
SELECT
    id,
    session_id,
    user_id,
    trigger_type,
    trigger_ref_id,
    strategy_type,
    tone_mode,
    intensity,
    purpose,
    score,
    message_text,
    COALESCE(seed_key, ''),
    channel,
    COALESCE(external_message_id, ''),
    COALESCE(telegram_message_id, 0),
    COALESCE(sent_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    delivery_status,
    user_replied,
    COALESCE(reply_delay_sec, 0),
    COALESCE(reply_sentiment, ''),
    COALESCE(reply_length, 0),
    COALESCE(followup_turn_count, 0),
    created_at,
    updated_at
FROM tg_proactive_messages
WHERE session_id = $1
ORDER BY COALESCE(sent_at, created_at) DESC, id DESC
LIMIT $2
`

	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]model.ProactiveMessage, 0, limit)
	for rows.Next() {
		var message model.ProactiveMessage
		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.UserID,
			&message.TriggerType,
			&message.TriggerRefID,
			&message.StrategyType,
			&message.ToneMode,
			&message.Intensity,
			&message.Purpose,
			&message.Score,
			&message.MessageText,
			&message.SeedKey,
			&message.Channel,
			&message.ExternalMessageID,
			&message.TelegramMessageID,
			&message.SentAt,
			&message.DeliveryStatus,
			&message.UserReplied,
			&message.ReplyDelaySec,
			&message.ReplySentiment,
			&message.ReplyLength,
			&message.FollowupTurnCount,
			&message.CreatedAt,
			&message.UpdatedAt,
		); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (s *Store) ListDueMemoryEvents(ctx context.Context, from time.Time, to time.Time, limit int) ([]model.MemoryEvent, error) {
	const query = `
SELECT
    id,
    session_id,
    COALESCE(message_id, 0),
    event_type,
    event_subtype,
    event_value,
    event_time,
    priority,
    used_for_proactive,
    created_at,
    updated_at
FROM tg_memory_events
WHERE used_for_proactive = FALSE
  AND event_time BETWEEN $1 AND $2
ORDER BY priority DESC, event_time ASC, id ASC
LIMIT $3
`

	rows, err := s.pool.Query(ctx, query, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]model.MemoryEvent, 0, limit)
	for rows.Next() {
		var event model.MemoryEvent
		if err := rows.Scan(
			&event.ID,
			&event.SessionID,
			&event.MessageID,
			&event.EventType,
			&event.EventSubtype,
			&event.EventValue,
			&event.EventTime,
			&event.Priority,
			&event.UsedForProactive,
			&event.CreatedAt,
			&event.UpdatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s *Store) ListConversationMessages(ctx context.Context, sessionID int64, limit int) ([]model.Message, error) {
	return s.ListRecentMessages(ctx, sessionID, limit)
}

func (s *Store) ListActiveSessionsForProactiveScan(ctx context.Context, limit int) ([]model.Session, error) {
	const query = `
SELECT
    id,
    user_id,
    session_status,
    mode,
    recent_turn_limit,
    last_message_at,
    COALESCE(last_user_message_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(last_bot_message_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(last_proactive_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(last_user_reply_to_proactive_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    consecutive_proactive_ignored,
    COALESCE(relationship_score, 40.00),
    current_mood,
    proactive_opt_in,
    COALESCE(quiet_hours_json, '[]'::jsonb),
    created_at,
    updated_at
FROM tg_chat_sessions
WHERE session_status = 'active'
  AND proactive_opt_in = TRUE
ORDER BY last_message_at DESC, id DESC
LIMIT $1
`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]model.Session, 0, limit)
	for rows.Next() {
		var session model.Session
		if err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.SessionStatus,
			&session.Mode,
			&session.RecentTurnLimit,
			&session.LastMessageAt,
			&session.LastUserMessageAt,
			&session.LastBotMessageAt,
			&session.LastProactiveAt,
			&session.LastUserReplyToProactiveAt,
			&session.ConsecutiveProactiveIgnored,
			&session.RelationshipScore,
			&session.CurrentMood,
			&session.ProactiveOptIn,
			&session.QuietHoursJSON,
			&session.CreatedAt,
			&session.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (s *Store) ListActiveProactiveSessions(ctx context.Context, botID string, channelName string, limit int) ([]model.ProactiveSession, error) {
	const query = `
SELECT
    u.id,
    u.bot_id,
    u.channel,
    u.external_user_id,
    u.external_chat_id,
    u.telegram_user_id,
    u.telegram_chat_id,
    u.username,
    u.first_name,
    u.created_at,
    u.updated_at,
    s.id,
    s.user_id,
    s.session_status,
    s.mode,
    s.recent_turn_limit,
    s.last_message_at,
    COALESCE(s.last_user_message_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(s.last_bot_message_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(s.last_proactive_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(s.last_user_reply_to_proactive_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    s.consecutive_proactive_ignored,
    COALESCE(s.relationship_score, 40.00),
    s.current_mood,
    s.proactive_opt_in,
    COALESCE(s.quiet_hours_json, '[]'::jsonb),
    s.created_at,
    s.updated_at
FROM tg_chat_sessions s
JOIN tg_users u ON u.id = s.user_id
WHERE s.session_status = 'active'
  AND s.proactive_opt_in = TRUE
  AND u.bot_id = $2
  AND u.channel = $3
ORDER BY s.last_message_at DESC, s.id DESC
LIMIT $1
`

	rows, err := s.pool.Query(ctx, query, limit, normalizedBotID(botID), normalizedChannel(channelName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]model.ProactiveSession, 0, limit)
	for rows.Next() {
		var item model.ProactiveSession
		if err := rows.Scan(
			&item.User.ID,
			&item.User.BotID,
			&item.User.Channel,
			&item.User.ExternalUserID,
			&item.User.ExternalChatID,
			&item.User.TelegramUserID,
			&item.User.TelegramChatID,
			&item.User.Username,
			&item.User.FirstName,
			&item.User.CreatedAt,
			&item.User.UpdatedAt,
			&item.Session.ID,
			&item.Session.UserID,
			&item.Session.SessionStatus,
			&item.Session.Mode,
			&item.Session.RecentTurnLimit,
			&item.Session.LastMessageAt,
			&item.Session.LastUserMessageAt,
			&item.Session.LastBotMessageAt,
			&item.Session.LastProactiveAt,
			&item.Session.LastUserReplyToProactiveAt,
			&item.Session.ConsecutiveProactiveIgnored,
			&item.Session.RelationshipScore,
			&item.Session.CurrentMood,
			&item.Session.ProactiveOptIn,
			&item.Session.QuietHoursJSON,
			&item.Session.CreatedAt,
			&item.Session.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}
