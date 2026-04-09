package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	redistore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/redis"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

type Config struct {
	PostgresDSN string
	RedisURL    string
}

type ConversationContext struct {
	User               model.User
	Session            model.Session
	RecentConversation []ollama.Message
}

type Manager struct {
	postgres *pgstore.Store
	redis    *redistore.Store
	logger   *slog.Logger
}

type ResetResult struct {
	HadData    bool
	SessionIDs []int64
}

func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Manager, error) {
	postgresStore, err := pgstore.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	redisStore, err := redistore.New(ctx, cfg.RedisURL)
	if err != nil {
		postgresStore.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	manager := &Manager{
		postgres: postgresStore,
		redis:    redisStore,
		logger:   logger,
	}

	if err := manager.postgres.EnsureSchema(ctx); err != nil {
		manager.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return manager, nil
}

func (m *Manager) Close() {
	if m == nil {
		return
	}

	if m.postgres != nil {
		m.postgres.Close()
	}
	if m.redis != nil {
		_ = m.redis.Close()
	}
}

func (m *Manager) BootstrapContext(ctx context.Context, message telegram.Message, defaultMode string, recentTurnLimit int) (ConversationContext, error) {
	if message.From == nil {
		return ConversationContext{}, fmt.Errorf("telegram message has no sender")
	}

	user, err := m.postgres.UpsertUser(ctx, pgstore.UpsertUserParams{
		TelegramUserID: message.From.ID,
		TelegramChatID: message.Chat.ID,
		Username:       message.From.Username,
		FirstName:      message.From.FirstName,
	})
	if err != nil {
		return ConversationContext{}, err
	}

	session, err := m.postgres.GetOrCreateActiveSession(ctx, user.ID, defaultMode, recentTurnLimit)
	if err != nil {
		return ConversationContext{}, err
	}

	recentConversation, err := m.loadRecentConversation(ctx, session.ID, recentTurnLimit)
	if err != nil {
		return ConversationContext{}, err
	}

	return ConversationContext{
		User:               user,
		Session:            session,
		RecentConversation: recentConversation,
	}, nil
}

func (m *Manager) SaveTurn(ctx context.Context, sessionID int64, role string, content string, telegramMessageID int64, updateID int64, mode string, recentTurnLimit int) error {
	content = strings.TrimSpace(content)
	if sessionID == 0 || content == "" {
		return nil
	}

	if _, err := m.postgres.InsertMessage(ctx, pgstore.InsertMessageParams{
		SessionID:         sessionID,
		TelegramMessageID: telegramMessageID,
		TelegramUpdateID:  updateID,
		Role:              role,
		Content:           content,
		Mode:              mode,
	}); err != nil {
		return err
	}

	if err := m.redis.AppendRecent(ctx, sessionID, redistore.Turn{
		Role:    role,
		Content: content,
		SavedAt: time.Now(),
	}, recentTurnLimit); err != nil {
		m.logger.Warn("failed to update recent chat cache", "session_id", sessionID, "error", err)
	}

	return nil
}

func (m *Manager) ResetConversation(ctx context.Context, telegramUserID int64) (ResetResult, error) {
	if telegramUserID == 0 {
		return ResetResult{}, nil
	}

	sessionIDs, err := m.postgres.ListSessionIDsByTelegramUser(ctx, telegramUserID)
	if err != nil {
		return ResetResult{}, err
	}

	deleted, err := m.postgres.DeleteConversationByTelegramUser(ctx, telegramUserID)
	if err != nil {
		return ResetResult{}, err
	}

	if err := m.redis.DeleteRecent(ctx, sessionIDs); err != nil {
		m.logger.Warn("failed to clear redis recent cache during reset", "telegram_user_id", telegramUserID, "error", err)
	}

	return ResetResult{
		HadData:    deleted || len(sessionIDs) > 0,
		SessionIDs: sessionIDs,
	}, nil
}

func (m *Manager) loadRecentConversation(ctx context.Context, sessionID int64, recentTurnLimit int) ([]ollama.Message, error) {
	turns, err := m.redis.GetRecent(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if len(turns) == 0 {
		messages, err := m.postgres.ListRecentMessages(ctx, sessionID, recentTurnLimit)
		if err != nil {
			return nil, err
		}

		if len(messages) == 0 {
			return nil, nil
		}

		turns = make([]redistore.Turn, 0, len(messages))
		for _, message := range messages {
			turns = append(turns, redistore.Turn{
				Role:    message.Role,
				Content: message.Content,
				SavedAt: message.CreatedAt,
			})
			if err := m.redis.AppendRecent(ctx, sessionID, redistore.Turn{
				Role:    message.Role,
				Content: message.Content,
				SavedAt: message.CreatedAt,
			}, recentTurnLimit); err != nil {
				m.logger.Warn("failed to warm recent chat cache", "session_id", sessionID, "error", err)
				break
			}
		}
	}

	recentConversation := make([]ollama.Message, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		recentConversation = append(recentConversation, ollama.Message{
			Role:    turn.Role,
			Content: turn.Content,
		})
	}

	return recentConversation, nil
}
