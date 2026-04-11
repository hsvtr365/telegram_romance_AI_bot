package postgres

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type UpsertUserParams struct {
	TelegramUserID int64
	TelegramChatID int64
	Username       string
	FirstName      string
}

type InsertMessageParams struct {
	SessionID         int64
	TelegramMessageID int64
	TelegramUpdateID  int64
	Role              string
	Content           string
	Mode              string
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS tg_users (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL UNIQUE,
    telegram_chat_id BIGINT NOT NULL UNIQUE,
    username VARCHAR(255),
    first_name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tg_chat_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES tg_users(id) ON DELETE CASCADE,
    session_status VARCHAR(32) NOT NULL DEFAULT 'active',
    mode VARCHAR(32) NOT NULL DEFAULT 'spicy',
    recent_turn_limit SMALLINT NOT NULL DEFAULT 14,
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_user_message_at TIMESTAMPTZ,
    last_bot_message_at TIMESTAMPTZ,
    last_proactive_at TIMESTAMPTZ,
    last_user_reply_to_proactive_at TIMESTAMPTZ,
    consecutive_proactive_ignored INT NOT NULL DEFAULT 0,
    relationship_score NUMERIC(5,2) NOT NULL DEFAULT 40.00,
    current_mood VARCHAR(32) NOT NULL DEFAULT 'neutral',
    conversation_phase VARCHAR(16) NOT NULL DEFAULT 'neutral',
    sexual_pause_until_turn INT NOT NULL DEFAULT 0,
    proactive_opt_in BOOLEAN NOT NULL DEFAULT TRUE,
    quiet_hours_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_status)
);

ALTER TABLE tg_chat_sessions
    ADD COLUMN IF NOT EXISTS last_user_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_bot_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_proactive_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_user_reply_to_proactive_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS consecutive_proactive_ignored INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS relationship_score NUMERIC(5,2) NOT NULL DEFAULT 40.00,
    ADD COLUMN IF NOT EXISTS current_mood VARCHAR(32) NOT NULL DEFAULT 'neutral',
    ADD COLUMN IF NOT EXISTS conversation_phase VARCHAR(16) NOT NULL DEFAULT 'neutral',
    ADD COLUMN IF NOT EXISTS sexual_pause_until_turn INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS proactive_opt_in BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS quiet_hours_json JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE tg_chat_sessions
    ALTER COLUMN proactive_opt_in SET DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_tg_chat_sessions_user_id ON tg_chat_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_tg_chat_sessions_last_message_at ON tg_chat_sessions (last_message_at DESC);
CREATE INDEX IF NOT EXISTS idx_tg_chat_sessions_proactive_scan
    ON tg_chat_sessions (proactive_opt_in, last_message_at DESC, last_proactive_at DESC);

CREATE TABLE IF NOT EXISTS tg_chat_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    telegram_message_id BIGINT,
    telegram_update_id BIGINT,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    content_len INT NOT NULL DEFAULT 0,
    mode VARCHAR(32) NOT NULL DEFAULT 'spicy',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tg_chat_messages_session_id_id
    ON tg_chat_messages (session_id, id DESC);

CREATE TABLE IF NOT EXISTS tg_memory_events (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    message_id BIGINT REFERENCES tg_chat_messages(id) ON DELETE SET NULL,
    event_type VARCHAR(32) NOT NULL,
    event_subtype VARCHAR(64) NOT NULL,
    event_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    event_time TIMESTAMPTZ NOT NULL,
    priority SMALLINT NOT NULL DEFAULT 50,
    used_for_proactive BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, event_type, event_subtype, event_time)
);

CREATE INDEX IF NOT EXISTS idx_tg_memory_events_session_time
    ON tg_memory_events (session_id, event_time DESC);

CREATE INDEX IF NOT EXISTS idx_tg_memory_events_proactive_scan
    ON tg_memory_events (used_for_proactive, priority DESC, event_time ASC);

CREATE TABLE IF NOT EXISTS tg_proactive_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES tg_users(id) ON DELETE CASCADE,
    trigger_type VARCHAR(32) NOT NULL,
    trigger_ref_id VARCHAR(128) NOT NULL,
    strategy_type VARCHAR(32) NOT NULL,
    tone_mode VARCHAR(16) NOT NULL,
    intensity VARCHAR(16) NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    score NUMERIC(6,2) NOT NULL,
    message_text TEXT NOT NULL,
    seed_key VARCHAR(128),
    telegram_message_id BIGINT,
    sent_at TIMESTAMPTZ,
    delivery_status VARCHAR(32) NOT NULL DEFAULT 'queued',
    user_replied BOOLEAN NOT NULL DEFAULT FALSE,
    reply_delay_sec INT,
    reply_sentiment VARCHAR(16),
    reply_length INT,
    followup_turn_count INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, trigger_type, trigger_ref_id)
);

CREATE INDEX IF NOT EXISTS idx_tg_proactive_messages_session_sent
    ON tg_proactive_messages (session_id, sent_at DESC);

CREATE INDEX IF NOT EXISTS idx_tg_proactive_messages_delivery
    ON tg_proactive_messages (delivery_status, sent_at ASC);

CREATE INDEX IF NOT EXISTS idx_tg_proactive_messages_type_sent
    ON tg_proactive_messages (trigger_type, sent_at DESC);

CREATE TABLE IF NOT EXISTS tg_proactive_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES tg_users(id) ON DELETE CASCADE,
    preferred_types_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    disliked_types_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    best_time_windows_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    max_per_day SMALLINT NOT NULL DEFAULT 1,
    max_per_week SMALLINT NOT NULL DEFAULT 4,
    min_gap_hours SMALLINT NOT NULL DEFAULT 20,
    avg_reply_delay_sec INT,
    proactive_success_score NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    timezone_name VARCHAR(64) NOT NULL DEFAULT 'Asia/Seoul',
    weight_overrides_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tg_user_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES tg_users(id) ON DELETE CASCADE,
    name_value TEXT,
    name_confirmed_at TIMESTAMPTZ,
    gender_value TEXT,
    gender_confirmed_at TIMESTAMPTZ,
    age_value TEXT,
    age_confirmed_at TIMESTAMPTZ,
    job_value TEXT,
    job_confirmed_at TIMESTAMPTZ,
    current_focus_value TEXT,
    current_focus_confirmed_at TIMESTAMPTZ,
    hobby_value TEXT,
    hobby_confirmed_at TIMESTAMPTZ,
    location_value TEXT,
    location_confirmed_at TIMESTAMPTZ,
    affiliation_value TEXT,
    affiliation_confirmed_at TIMESTAMPTZ,
    last_requested_slot VARCHAR(32),
    last_requested_user_turn_count INT NOT NULL DEFAULT 0,
    collection_paused_until_turn INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tg_user_profiles
    ADD COLUMN IF NOT EXISTS name_value TEXT,
    ADD COLUMN IF NOT EXISTS name_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS gender_value TEXT,
    ADD COLUMN IF NOT EXISTS gender_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS age_value TEXT,
    ADD COLUMN IF NOT EXISTS age_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS job_value TEXT,
    ADD COLUMN IF NOT EXISTS job_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS current_focus_value TEXT,
    ADD COLUMN IF NOT EXISTS current_focus_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hobby_value TEXT,
    ADD COLUMN IF NOT EXISTS hobby_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS location_value TEXT,
    ADD COLUMN IF NOT EXISTS location_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS affiliation_value TEXT,
    ADD COLUMN IF NOT EXISTS affiliation_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_requested_slot VARCHAR(32),
    ADD COLUMN IF NOT EXISTS last_requested_user_turn_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS collection_paused_until_turn INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE TABLE IF NOT EXISTS tg_user_traits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES tg_users(id) ON DELETE CASCADE,
    trait_type VARCHAR(32) NOT NULL,
    normalized_value VARCHAR(128) NOT NULL,
    display_value VARCHAR(128) NOT NULL,
    source_text TEXT,
    last_confirmed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, normalized_value)
);

ALTER TABLE tg_user_traits
    ADD COLUMN IF NOT EXISTS trait_type VARCHAR(32) NOT NULL DEFAULT 'avoid',
    ADD COLUMN IF NOT EXISTS normalized_value VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS display_value VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_text TEXT,
    ADD COLUMN IF NOT EXISTS last_confirmed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_tg_user_traits_user_id_updated
    ON tg_user_traits (user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS tg_special_days (
    id BIGSERIAL PRIMARY KEY,
    day DATE NOT NULL,
    name VARCHAR(128) NOT NULL,
    kind_code VARCHAR(8) NOT NULL,
    kind_label VARCHAR(32) NOT NULL,
    is_holiday BOOLEAN NOT NULL DEFAULT FALSE,
    seq SMALLINT NOT NULL DEFAULT 0,
    is_major_holiday BOOLEAN NOT NULL DEFAULT FALSE,
    major_holiday_group VARCHAR(32),
    source_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (day, kind_code, seq, name)
);

CREATE INDEX IF NOT EXISTS idx_tg_special_days_day
    ON tg_special_days (day ASC, kind_code ASC, seq ASC);

CREATE INDEX IF NOT EXISTS idx_tg_special_days_major
    ON tg_special_days (is_major_holiday, day ASC, major_holiday_group ASC);

CREATE TABLE IF NOT EXISTS tg_session_prompt_topics (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    topic_key VARCHAR(128) NOT NULL,
    topic_type VARCHAR(32) NOT NULL,
    topic_date DATE NOT NULL,
    source VARCHAR(16) NOT NULL,
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, topic_key)
);

CREATE INDEX IF NOT EXISTS idx_tg_session_prompt_topics_session
    ON tg_session_prompt_topics (session_id ASC, used_at DESC);

CREATE TABLE IF NOT EXISTS tg_topic_slots (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    slot_key VARCHAR(128) NOT NULL,
    topic_label VARCHAR(255) NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'watch',
    importance SMALLINT NOT NULL DEFAULT 50,
    confidence VARCHAR(16) NOT NULL DEFAULT 'low',
    source_kind VARCHAR(32) NOT NULL DEFAULT 'memory_slot_ai',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_source_message_id BIGINT REFERENCES tg_chat_messages(id) ON DELETE SET NULL,
    mention_count INT NOT NULL DEFAULT 1,
    evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, slot_key)
);

CREATE INDEX IF NOT EXISTS idx_tg_topic_slots_session_status
    ON tg_topic_slots (session_id ASC, status ASC, importance DESC, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_tg_topic_slots_message
    ON tg_topic_slots (last_source_message_id ASC);

CREATE TABLE IF NOT EXISTS tg_conversation_state_slots (
    session_id BIGINT PRIMARY KEY REFERENCES tg_chat_sessions(id) ON DELETE CASCADE,
    current_stage VARCHAR(32) NOT NULL DEFAULT '',
    stage_direction VARCHAR(32) NOT NULL DEFAULT '',
    emotional_tone VARCHAR(32) NOT NULL DEFAULT '',
    interaction_mode VARCHAR(32) NOT NULL DEFAULT '',
    open_loop_summary TEXT NOT NULL DEFAULT '',
    focus_topic_key VARCHAR(128) NOT NULL DEFAULT '',
    confidence VARCHAR(16) NOT NULL DEFAULT 'low',
    evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_source_message_id BIGINT REFERENCES tg_chat_messages(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tg_conversation_state_slots_message
    ON tg_conversation_state_slots (last_source_message_id ASC);
`

	_, err := s.pool.Exec(ctx, schema)
	return err
}

func (s *Store) UpsertUser(ctx context.Context, params UpsertUserParams) (model.User, error) {
	const query = `
INSERT INTO tg_users (
    telegram_user_id,
    telegram_chat_id,
    username,
    first_name
) VALUES ($1, $2, $3, $4)
ON CONFLICT (telegram_user_id) DO UPDATE
SET
    telegram_chat_id = EXCLUDED.telegram_chat_id,
    username = EXCLUDED.username,
    first_name = EXCLUDED.first_name,
    updated_at = NOW()
RETURNING id, telegram_user_id, telegram_chat_id, username, first_name, created_at, updated_at
`

	var user model.User
	err := s.pool.QueryRow(
		ctx,
		query,
		params.TelegramUserID,
		params.TelegramChatID,
		params.Username,
		params.FirstName,
	).Scan(
		&user.ID,
		&user.TelegramUserID,
		&user.TelegramChatID,
		&user.Username,
		&user.FirstName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	return user, err
}

func (s *Store) GetOrCreateActiveSession(ctx context.Context, userID int64, mode string, recentTurnLimit int) (model.Session, error) {
	const query = `
INSERT INTO tg_chat_sessions (
    user_id,
    session_status,
    mode,
    recent_turn_limit,
    last_message_at,
    last_user_message_at,
    consecutive_proactive_ignored,
    relationship_score,
    current_mood,
    conversation_phase,
    sexual_pause_until_turn,
    proactive_opt_in,
    quiet_hours_json
) VALUES ($1, 'active', $2, $3, NOW(), NOW(), 0, 40.00, 'neutral', 'neutral', 0, TRUE, '[]'::jsonb)
ON CONFLICT (user_id, session_status) DO UPDATE
SET
    last_message_at = NOW(),
    last_user_message_at = NOW(),
    updated_at = NOW()
RETURNING
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
    conversation_phase,
    COALESCE(sexual_pause_until_turn, 0),
    proactive_opt_in,
    COALESCE(quiet_hours_json, '[]'::jsonb),
    created_at,
    updated_at
`

	var session model.Session
	err := s.pool.QueryRow(ctx, query, userID, mode, recentTurnLimit).Scan(
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
		&session.ConversationPhase,
		&session.SexualPauseUntilTurn,
		&session.ProactiveOptIn,
		&session.QuietHoursJSON,
		&session.CreatedAt,
		&session.UpdatedAt,
	)

	return session, err
}

func (s *Store) InsertMessage(ctx context.Context, params InsertMessageParams) (model.Message, error) {
	const query = `
INSERT INTO tg_chat_messages (
    session_id,
    telegram_message_id,
    telegram_update_id,
    role,
    content,
    content_len,
    mode
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, session_id, role, content, mode, created_at
`

	var message model.Message
	err := s.pool.QueryRow(
		ctx,
		query,
		params.SessionID,
		nullableInt64(params.TelegramMessageID),
		nullableInt64(params.TelegramUpdateID),
		params.Role,
		params.Content,
		utf8.RuneCountInString(params.Content),
		params.Mode,
	).Scan(
		&message.ID,
		&message.SessionID,
		&message.Role,
		&message.Content,
		&message.Mode,
		&message.CreatedAt,
	)

	return message, err
}

func (s *Store) TouchSessionMessage(ctx context.Context, sessionID int64, role string, at time.Time) error {
	const query = `
UPDATE tg_chat_sessions
SET
    last_message_at = $2,
    last_user_message_at = CASE WHEN $3 = 'user' THEN $2 ELSE last_user_message_at END,
    last_bot_message_at = CASE WHEN $3 IN ('assistant', 'bot') THEN $2 ELSE last_bot_message_at END,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, at, role)
	return err
}

func (s *Store) MarkSessionProactiveSent(ctx context.Context, sessionID int64, sentAt time.Time) error {
	const query = `
UPDATE tg_chat_sessions
SET
    last_proactive_at = $2,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, sentAt)
	return err
}

func (s *Store) MarkSessionProactiveReplied(ctx context.Context, sessionID int64, repliedAt time.Time) error {
	const query = `
UPDATE tg_chat_sessions
SET
    last_user_reply_to_proactive_at = $2,
    consecutive_proactive_ignored = 0,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, repliedAt)
	return err
}

func (s *Store) IncrementSessionProactiveIgnored(ctx context.Context, sessionID int64) error {
	const query = `
UPDATE tg_chat_sessions
SET
    consecutive_proactive_ignored = consecutive_proactive_ignored + 1,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID)
	return err
}

func (s *Store) SetSessionConsecutiveProactiveIgnored(ctx context.Context, sessionID int64, count int) error {
	const query = `
UPDATE tg_chat_sessions
SET
    consecutive_proactive_ignored = GREATEST($2, 0),
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, count)
	return err
}

func (s *Store) SetSessionProactiveOptIn(ctx context.Context, sessionID int64, optIn bool) error {
	const query = `
UPDATE tg_chat_sessions
SET
    proactive_opt_in = $2,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, optIn)
	return err
}

func (s *Store) SetSessionQuietHours(ctx context.Context, sessionID int64, quietHoursJSON []byte) error {
	const query = `
UPDATE tg_chat_sessions
SET
    quiet_hours_json = COALESCE($2::jsonb, '[]'::jsonb),
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, quietHoursJSON)
	return err
}

func (s *Store) SetSessionCurrentMood(ctx context.Context, sessionID int64, mood string) error {
	const query = `
UPDATE tg_chat_sessions
SET
    current_mood = $2,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, mood)
	return err
}

func (s *Store) SetSessionConversationPhase(ctx context.Context, sessionID int64, phase string) error {
	const query = `
UPDATE tg_chat_sessions
SET
    conversation_phase = $2,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, phase)
	return err
}

func (s *Store) SetSessionSexualPauseUntilTurn(ctx context.Context, sessionID int64, untilTurn int) error {
	const query = `
UPDATE tg_chat_sessions
SET
    sexual_pause_until_turn = GREATEST($2, 0),
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, untilTurn)
	return err
}

func (s *Store) SetSessionRelationshipScore(ctx context.Context, sessionID int64, score float64) error {
	const query = `
UPDATE tg_chat_sessions
SET
    relationship_score = $2,
    updated_at = NOW()
WHERE id = $1
`

	_, err := s.pool.Exec(ctx, query, sessionID, score)
	return err
}

func (s *Store) ListRecentMessages(ctx context.Context, sessionID int64, limit int) ([]model.Message, error) {
	const query = `
SELECT id, session_id, role, content, mode, created_at
FROM (
    SELECT id, session_id, role, content, mode, created_at
    FROM tg_chat_messages
    WHERE session_id = $1
    ORDER BY id DESC
    LIMIT $2
) recent
ORDER BY id ASC
`

	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]model.Message, 0, limit)
	for rows.Next() {
		var message model.Message
		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.Role,
			&message.Content,
			&message.Mode,
			&message.CreatedAt,
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

func (s *Store) ListSessionIDsByTelegramUser(ctx context.Context, telegramUserID int64) ([]int64, error) {
	const query = `
SELECT s.id
FROM tg_chat_sessions s
JOIN tg_users u ON u.id = s.user_id
WHERE u.telegram_user_id = $1
ORDER BY s.id ASC
`

	rows, err := s.pool.Query(ctx, query, telegramUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessionIDs := make([]int64, 0, 4)
	for rows.Next() {
		var sessionID int64
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		sessionIDs = append(sessionIDs, sessionID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sessionIDs, nil
}

func (s *Store) DeleteConversationByTelegramUser(ctx context.Context, telegramUserID int64) (bool, error) {
	const query = `
DELETE FROM tg_users
WHERE telegram_user_id = $1
`

	tag, err := s.pool.Exec(ctx, query, telegramUserID)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() > 0, nil
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (s *Store) String() string {
	return fmt.Sprintf("postgres_store{%p}", s)
}
