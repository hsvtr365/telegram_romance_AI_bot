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
	UserProfile        model.UserProfile
	UserTraits         []model.UserTrait
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

	userProfile, err := m.postgres.GetUserProfileByUserID(ctx, user.ID)
	if err != nil {
		return ConversationContext{}, err
	}

	userTraits, err := m.postgres.ListUserTraitsByUserID(ctx, user.ID, 32)
	if err != nil {
		return ConversationContext{}, err
	}

	return ConversationContext{
		User:               user,
		UserProfile:        userProfile,
		UserTraits:         userTraits,
		Session:            session,
		RecentConversation: recentConversation,
	}, nil
}

func (m *Manager) SaveTurn(ctx context.Context, sessionID int64, role string, content string, telegramMessageID int64, updateID int64, mode string, recentTurnLimit int) error {
	content = strings.TrimSpace(content)
	if sessionID == 0 || content == "" {
		return nil
	}

	now := time.Now()

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

	if err := m.postgres.TouchSessionMessage(ctx, sessionID, role, now); err != nil {
		m.logger.Warn("failed to update session activity", "session_id", sessionID, "role", role, "error", err)
	}

	if err := m.redis.AppendRecent(ctx, sessionID, redistore.Turn{
		Role:    role,
		Content: content,
		SavedAt: now,
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

	if err := m.redis.DeleteProactiveSessionData(ctx, sessionIDs); err != nil {
		m.logger.Warn("failed to clear redis proactive cache during reset", "telegram_user_id", telegramUserID, "error", err)
	}

	return ResetResult{
		HadData:    deleted || len(sessionIDs) > 0,
		SessionIDs: sessionIDs,
	}, nil
}

func (m *Manager) TouchSessionMessage(ctx context.Context, sessionID int64, role string, at time.Time) error {
	return m.postgres.TouchSessionMessage(ctx, sessionID, role, at)
}

func (m *Manager) MarkSessionProactiveSent(ctx context.Context, sessionID int64, sentAt time.Time) error {
	return m.postgres.MarkSessionProactiveSent(ctx, sessionID, sentAt)
}

func (m *Manager) MarkSessionProactiveReplied(ctx context.Context, sessionID int64, repliedAt time.Time) error {
	return m.postgres.MarkSessionProactiveReplied(ctx, sessionID, repliedAt)
}

func (m *Manager) IncrementSessionProactiveIgnored(ctx context.Context, sessionID int64) error {
	return m.postgres.IncrementSessionProactiveIgnored(ctx, sessionID)
}

func (m *Manager) SetSessionConsecutiveProactiveIgnored(ctx context.Context, sessionID int64, count int) error {
	return m.postgres.SetSessionConsecutiveProactiveIgnored(ctx, sessionID, count)
}

func (m *Manager) SetSessionProactiveOptIn(ctx context.Context, sessionID int64, optIn bool) error {
	return m.postgres.SetSessionProactiveOptIn(ctx, sessionID, optIn)
}

func (m *Manager) SetSessionQuietHours(ctx context.Context, sessionID int64, quietHoursJSON []byte) error {
	return m.postgres.SetSessionQuietHours(ctx, sessionID, quietHoursJSON)
}

func (m *Manager) SetSessionCurrentMood(ctx context.Context, sessionID int64, mood string) error {
	return m.postgres.SetSessionCurrentMood(ctx, sessionID, mood)
}

func (m *Manager) SetSessionConversationPhase(ctx context.Context, sessionID int64, phase string) error {
	return m.postgres.SetSessionConversationPhase(ctx, sessionID, phase)
}

func (m *Manager) SetSessionSexualPauseUntilTurn(ctx context.Context, sessionID int64, untilTurn int) error {
	return m.postgres.SetSessionSexualPauseUntilTurn(ctx, sessionID, untilTurn)
}

func (m *Manager) SetSessionRelationshipScore(ctx context.Context, sessionID int64, score float64) error {
	return m.postgres.SetSessionRelationshipScore(ctx, sessionID, score)
}

func (m *Manager) GetUserProfileByUserID(ctx context.Context, userID int64) (model.UserProfile, error) {
	return m.postgres.GetUserProfileByUserID(ctx, userID)
}

func (m *Manager) UpsertUserProfile(ctx context.Context, params pgstore.UpsertUserProfileParams) (model.UserProfile, error) {
	return m.postgres.UpsertUserProfile(ctx, params)
}

func (m *Manager) UpsertUserTrait(ctx context.Context, params pgstore.UpsertUserTraitParams) (model.UserTrait, error) {
	return m.postgres.UpsertUserTrait(ctx, params)
}

func (m *Manager) ListUserTraitsByUserID(ctx context.Context, userID int64, limit int) ([]model.UserTrait, error) {
	return m.postgres.ListUserTraitsByUserID(ctx, userID, limit)
}

func (m *Manager) MarkUserProfileSlotPrompted(ctx context.Context, userID int64, slot string, userTurnCount int) error {
	return m.postgres.MarkUserProfileSlotPrompted(ctx, userID, slot, userTurnCount)
}

func (m *Manager) PauseUserProfileCollection(ctx context.Context, userID int64, untilTurn int) error {
	return m.postgres.PauseUserProfileCollection(ctx, userID, untilTurn)
}

func (m *Manager) CountUserTurns(ctx context.Context, sessionID int64) (int, error) {
	return m.postgres.CountUserTurns(ctx, sessionID)
}

func (m *Manager) UpsertProactiveProfile(ctx context.Context, params pgstore.UpsertProactiveProfileParams) (model.ProactiveProfile, error) {
	return m.postgres.UpsertProactiveProfile(ctx, params)
}

func (m *Manager) GetProactiveProfileByUserID(ctx context.Context, userID int64) (model.ProactiveProfile, error) {
	return m.postgres.GetProactiveProfileByUserID(ctx, userID)
}

func (m *Manager) InsertMemoryEvent(ctx context.Context, params pgstore.InsertMemoryEventParams) (model.MemoryEvent, error) {
	return m.postgres.InsertMemoryEvent(ctx, params)
}

func (m *Manager) ListRecentMemoryEvents(ctx context.Context, sessionID int64, limit int) ([]model.MemoryEvent, error) {
	return m.postgres.ListRecentMemoryEvents(ctx, sessionID, limit)
}

func (m *Manager) ListDueMemoryEvents(ctx context.Context, from time.Time, to time.Time, limit int) ([]model.MemoryEvent, error) {
	return m.postgres.ListDueMemoryEvents(ctx, from, to, limit)
}

func (m *Manager) MarkMemoryEventUsedForProactive(ctx context.Context, eventID int64) error {
	return m.postgres.MarkMemoryEventUsedForProactive(ctx, eventID)
}

func (m *Manager) ReplaceSpecialDaysForMonthKind(ctx context.Context, year int, month time.Month, kindCode string, items []model.SpecialDay) error {
	return m.postgres.ReplaceSpecialDaysForMonthKind(ctx, year, month, kindCode, items)
}

func (m *Manager) ListSpecialDays(ctx context.Context, from time.Time, to time.Time) ([]model.SpecialDay, error) {
	return m.postgres.ListSpecialDays(ctx, from, to)
}

func (m *Manager) HasSessionPromptTopic(ctx context.Context, sessionID int64, topicKey string) (bool, error) {
	return m.postgres.HasSessionPromptTopic(ctx, sessionID, topicKey)
}

func (m *Manager) MarkSessionPromptTopicUsed(ctx context.Context, topic model.SessionPromptTopic) error {
	return m.postgres.MarkSessionPromptTopicUsed(ctx, topic)
}

func (m *Manager) InsertProactiveMessage(ctx context.Context, params pgstore.InsertProactiveMessageParams) (model.ProactiveMessage, error) {
	return m.postgres.InsertProactiveMessage(ctx, params)
}

func (m *Manager) UpdateProactiveMessageDelivery(ctx context.Context, messageID int64, telegramMessageID int64, sentAt time.Time, status string) error {
	return m.postgres.UpdateProactiveMessageDelivery(ctx, messageID, telegramMessageID, sentAt, status)
}

func (m *Manager) MarkProactiveMessageReplied(ctx context.Context, messageID int64, replyDelaySec int, replySentiment string, replyLength int, followupTurnCount int) error {
	return m.postgres.MarkProactiveMessageReplied(ctx, messageID, replyDelaySec, replySentiment, replyLength, followupTurnCount)
}

func (m *Manager) ListPendingProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]model.ProactiveMessage, error) {
	return m.postgres.ListPendingProactiveMessages(ctx, sessionID, limit)
}

func (m *Manager) ListRecentProactiveMessages(ctx context.Context, sessionID int64, limit int) ([]model.ProactiveMessage, error) {
	return m.postgres.ListRecentProactiveMessages(ctx, sessionID, limit)
}

func (m *Manager) ListActiveSessionsForProactiveScan(ctx context.Context, limit int) ([]model.Session, error) {
	return m.postgres.ListActiveSessionsForProactiveScan(ctx, limit)
}

func (m *Manager) ListActiveProactiveSessions(ctx context.Context, limit int) ([]model.ProactiveSession, error) {
	return m.postgres.ListActiveProactiveSessions(ctx, limit)
}

func (m *Manager) ListConversationMessages(ctx context.Context, sessionID int64, limit int) ([]model.Message, error) {
	return m.postgres.ListConversationMessages(ctx, sessionID, limit)
}

func (m *Manager) AcquireProactiveLock(ctx context.Context, sessionID int64, workerID string, ttl time.Duration) (bool, error) {
	return m.redis.AcquireProactiveLock(ctx, sessionID, workerID, ttl)
}

func (m *Manager) ReleaseProactiveLock(ctx context.Context, sessionID int64, workerID string) (bool, error) {
	return m.redis.ReleaseProactiveLock(ctx, sessionID, workerID)
}

func (m *Manager) SetProactiveCooldown(ctx context.Context, sessionID int64, cooldown redistore.ProactiveCooldown, ttl time.Duration) error {
	return m.redis.SetProactiveCooldown(ctx, sessionID, cooldown, ttl)
}

func (m *Manager) GetProactiveCooldown(ctx context.Context, sessionID int64) (redistore.ProactiveCooldown, bool, error) {
	return m.redis.GetProactiveCooldown(ctx, sessionID)
}

func (m *Manager) MarkProactiveTriggered(ctx context.Context, triggerType, refID string, ttl time.Duration) (bool, error) {
	return m.redis.MarkProactiveTriggered(ctx, triggerType, refID, ttl)
}

func (m *Manager) StoreProactiveCandidate(ctx context.Context, candidateID string, payload []byte, ttl time.Duration) error {
	return m.redis.StoreProactiveCandidate(ctx, candidateID, payload, ttl)
}

func (m *Manager) LoadProactiveCandidate(ctx context.Context, candidateID string) ([]byte, bool, error) {
	return m.redis.LoadProactiveCandidate(ctx, candidateID)
}

func (m *Manager) EnqueueProactiveQueueItem(ctx context.Context, bucket string, item redistore.ProactiveQueueItem, ttl time.Duration) error {
	return m.redis.EnqueueProactiveQueueItem(ctx, bucket, item, ttl)
}

func (m *Manager) ListDueProactiveQueueItems(ctx context.Context, bucket string, now time.Time, limit int) ([]redistore.ProactiveQueueItem, error) {
	return m.redis.ListDueProactiveQueueItems(ctx, bucket, now, limit)
}

func (m *Manager) RemoveProactiveQueueItem(ctx context.Context, bucket string, item redistore.ProactiveQueueItem) error {
	return m.redis.RemoveProactiveQueueItem(ctx, bucket, item)
}

func (m *Manager) SetProactivePresence(ctx context.Context, sessionID int64, presence redistore.ProactivePresence, ttl time.Duration) error {
	return m.redis.SetProactivePresence(ctx, sessionID, presence, ttl)
}

func (m *Manager) GetProactivePresence(ctx context.Context, sessionID int64) (redistore.ProactivePresence, bool, error) {
	return m.redis.GetProactivePresence(ctx, sessionID)
}

func (m *Manager) DeleteProactiveSessionData(ctx context.Context, sessionIDs []int64) error {
	return m.redis.DeleteProactiveSessionData(ctx, sessionIDs)
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
