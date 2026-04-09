package postgres

import (
	"context"
	"fmt"
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_status)
);

CREATE INDEX IF NOT EXISTS idx_tg_chat_sessions_user_id ON tg_chat_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_tg_chat_sessions_last_message_at ON tg_chat_sessions (last_message_at DESC);

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
    last_message_at
) VALUES ($1, 'active', $2, $3, NOW())
ON CONFLICT (user_id, session_status) DO UPDATE
SET
    last_message_at = NOW(),
    updated_at = NOW()
RETURNING id, user_id, session_status, mode, recent_turn_limit, last_message_at, created_at, updated_at
`

	var session model.Session
	err := s.pool.QueryRow(ctx, query, userID, mode, recentTurnLimit).Scan(
		&session.ID,
		&session.UserID,
		&session.SessionStatus,
		&session.Mode,
		&session.RecentTurnLimit,
		&session.LastMessageAt,
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
