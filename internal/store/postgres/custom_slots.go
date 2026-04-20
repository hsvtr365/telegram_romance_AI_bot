package postgres

import (
	"context"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func (s *Store) ListSessionCustomSlots(ctx context.Context, sessionID int64) ([]model.CustomSlot, error) {
	const query = `
SELECT id, session_id, slot_index, content, created_at
FROM tg_session_custom_slots
WHERE session_id = $1
ORDER BY slot_index ASC
`
	rows, err := s.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]model.CustomSlot, 0, 10)
	for rows.Next() {
		var slot model.CustomSlot
		if err := rows.Scan(
			&slot.ID,
			&slot.SessionID,
			&slot.SlotIndex,
			&slot.Content,
			&slot.CreatedAt,
		); err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

func (s *Store) UpsertSessionCustomSlot(ctx context.Context, sessionID int64, slotIndex int, content string) (model.CustomSlot, error) {
	const query = `
INSERT INTO tg_session_custom_slots (session_id, slot_index, content)
VALUES ($1, $2, $3)
ON CONFLICT (session_id, slot_index) DO UPDATE
SET content = EXCLUDED.content
RETURNING id, session_id, slot_index, content, created_at
`
	var slot model.CustomSlot
	err := s.pool.QueryRow(ctx, query, sessionID, slotIndex, content).Scan(
		&slot.ID,
		&slot.SessionID,
		&slot.SlotIndex,
		&slot.Content,
		&slot.CreatedAt,
	)
	return slot, err
}

func (s *Store) DeleteSessionCustomSlot(ctx context.Context, sessionID int64, slotIndex int) error {
	const query = `
DELETE FROM tg_session_custom_slots
WHERE session_id = $1 AND slot_index = $2
`
	_, err := s.pool.Exec(ctx, query, sessionID, slotIndex)
	return err
}
