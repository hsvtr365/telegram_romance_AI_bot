package store

import (
	"context"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func (m *Manager) ListSessionCustomSlots(ctx context.Context, sessionID int64) ([]model.CustomSlot, error) {
	return m.postgres.ListSessionCustomSlots(ctx, sessionID)
}

func (m *Manager) UpsertSessionCustomSlot(ctx context.Context, sessionID int64, slotIndex int, content string) (model.CustomSlot, error) {
	return m.postgres.UpsertSessionCustomSlot(ctx, sessionID, slotIndex, content)
}

func (m *Manager) DeleteSessionCustomSlot(ctx context.Context, sessionID int64, slotIndex int) error {
	return m.postgres.DeleteSessionCustomSlot(ctx, sessionID, slotIndex)
}
