package postgres

import (
	"context"
	"errors"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListActiveTopicSlots(ctx context.Context, sessionID int64, limit int) ([]model.TopicSlot, error) {
	if limit <= 0 {
		limit = 10
	}

	const query = `
SELECT
    id,
    session_id,
    slot_key,
    topic_label,
    summary,
    status,
    importance,
    confidence,
    source_kind,
    first_seen_at,
    last_seen_at,
    COALESCE(last_source_message_id, 0),
    mention_count,
    evidence_json,
    created_at,
    updated_at
FROM tg_topic_slots
WHERE session_id = $1
  AND status IN ('active', 'watch')
ORDER BY
    CASE status
        WHEN 'active' THEN 0
        WHEN 'watch' THEN 1
        ELSE 2
    END ASC,
    importance DESC,
    last_seen_at DESC,
    id DESC
LIMIT $2
`

	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]model.TopicSlot, 0, limit)
	for rows.Next() {
		var slot model.TopicSlot
		if err := rows.Scan(
			&slot.ID,
			&slot.SessionID,
			&slot.SlotKey,
			&slot.TopicLabel,
			&slot.Summary,
			&slot.Status,
			&slot.Importance,
			&slot.Confidence,
			&slot.SourceKind,
			&slot.FirstSeenAt,
			&slot.LastSeenAt,
			&slot.LastSourceMessageID,
			&slot.MentionCount,
			&slot.EvidenceJSON,
			&slot.CreatedAt,
			&slot.UpdatedAt,
		); err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return slots, nil
}

func (s *Store) UpsertTopicSlot(ctx context.Context, slot model.TopicSlot) (model.TopicSlot, error) {
	const query = `
INSERT INTO tg_topic_slots (
    session_id,
    slot_key,
    topic_label,
    summary,
    status,
    importance,
    confidence,
    source_kind,
    first_seen_at,
    last_seen_at,
    last_source_message_id,
    mention_count,
    evidence_json
) VALUES (
    $1,
    $2,
    $3,
    COALESCE(NULLIF($4, ''), ''),
    COALESCE(NULLIF($5, ''), 'watch'),
    GREATEST($6, 0),
    COALESCE(NULLIF($7, ''), 'low'),
    COALESCE(NULLIF($8, ''), 'memory_slot_ai'),
    COALESCE(NULLIF($9, TIMESTAMPTZ '0001-01-01 00:00:00+00'), NOW()),
    COALESCE(NULLIF($10, TIMESTAMPTZ '0001-01-01 00:00:00+00'), NOW()),
    NULLIF($11, 0),
    GREATEST($12, 1),
    COALESCE($13::jsonb, '[]'::jsonb)
)
ON CONFLICT (session_id, slot_key) DO UPDATE
SET
    topic_label = EXCLUDED.topic_label,
    summary = EXCLUDED.summary,
    status = EXCLUDED.status,
    importance = EXCLUDED.importance,
    confidence = EXCLUDED.confidence,
    source_kind = EXCLUDED.source_kind,
    first_seen_at = LEAST(tg_topic_slots.first_seen_at, EXCLUDED.first_seen_at),
    last_seen_at = EXCLUDED.last_seen_at,
    last_source_message_id = EXCLUDED.last_source_message_id,
    mention_count = GREATEST(EXCLUDED.mention_count, tg_topic_slots.mention_count),
    evidence_json = EXCLUDED.evidence_json,
    updated_at = NOW()
RETURNING
    id,
    session_id,
    slot_key,
    topic_label,
    summary,
    status,
    importance,
    confidence,
    source_kind,
    first_seen_at,
    last_seen_at,
    COALESCE(last_source_message_id, 0),
    mention_count,
    evidence_json,
    created_at,
    updated_at
`

	var updated model.TopicSlot
	err := s.pool.QueryRow(
		ctx,
		query,
		slot.SessionID,
		slot.SlotKey,
		slot.TopicLabel,
		slot.Summary,
		slot.Status,
		slot.Importance,
		slot.Confidence,
		slot.SourceKind,
		slot.FirstSeenAt,
		slot.LastSeenAt,
		slot.LastSourceMessageID,
		slot.MentionCount,
		slot.EvidenceJSON,
	).Scan(
		&updated.ID,
		&updated.SessionID,
		&updated.SlotKey,
		&updated.TopicLabel,
		&updated.Summary,
		&updated.Status,
		&updated.Importance,
		&updated.Confidence,
		&updated.SourceKind,
		&updated.FirstSeenAt,
		&updated.LastSeenAt,
		&updated.LastSourceMessageID,
		&updated.MentionCount,
		&updated.EvidenceJSON,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	return updated, err
}

func (s *Store) GetConversationStateSlot(ctx context.Context, sessionID int64) (model.ConversationStateSlot, error) {
	const query = `
SELECT
    session_id,
    COALESCE(current_stage, ''),
    COALESCE(stage_direction, ''),
    COALESCE(emotional_tone, ''),
    COALESCE(interaction_mode, ''),
    COALESCE(open_loop_summary, ''),
    COALESCE(focus_topic_key, ''),
    COALESCE(confidence, ''),
    COALESCE(evidence_json, '[]'::jsonb),
    COALESCE(last_source_message_id, 0),
    created_at,
    updated_at
FROM tg_conversation_state_slots
WHERE session_id = $1
`

	var slot model.ConversationStateSlot
	err := s.pool.QueryRow(ctx, query, sessionID).Scan(
		&slot.SessionID,
		&slot.CurrentStage,
		&slot.StageDirection,
		&slot.EmotionalTone,
		&slot.InteractionMode,
		&slot.OpenLoopSummary,
		&slot.FocusTopicKey,
		&slot.Confidence,
		&slot.EvidenceJSON,
		&slot.LastSourceMessageID,
		&slot.CreatedAt,
		&slot.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ConversationStateSlot{SessionID: sessionID}, nil
		}
		return model.ConversationStateSlot{}, err
	}
	return slot, nil
}

func (s *Store) UpsertConversationStateSlot(ctx context.Context, slot model.ConversationStateSlot) (model.ConversationStateSlot, error) {
	const query = `
INSERT INTO tg_conversation_state_slots (
    session_id,
    current_stage,
    stage_direction,
    emotional_tone,
    interaction_mode,
    open_loop_summary,
    focus_topic_key,
    confidence,
    evidence_json,
    last_source_message_id
) VALUES (
    $1,
    COALESCE(NULLIF($2, ''), ''),
    COALESCE(NULLIF($3, ''), ''),
    COALESCE(NULLIF($4, ''), ''),
    COALESCE(NULLIF($5, ''), ''),
    COALESCE(NULLIF($6, ''), ''),
    COALESCE(NULLIF($7, ''), ''),
    COALESCE(NULLIF($8, ''), 'low'),
    COALESCE($9::jsonb, '[]'::jsonb),
    NULLIF($10, 0)
)
ON CONFLICT (session_id) DO UPDATE
SET
    current_stage = EXCLUDED.current_stage,
    stage_direction = EXCLUDED.stage_direction,
    emotional_tone = EXCLUDED.emotional_tone,
    interaction_mode = EXCLUDED.interaction_mode,
    open_loop_summary = EXCLUDED.open_loop_summary,
    focus_topic_key = EXCLUDED.focus_topic_key,
    confidence = EXCLUDED.confidence,
    evidence_json = EXCLUDED.evidence_json,
    last_source_message_id = EXCLUDED.last_source_message_id,
    updated_at = NOW()
RETURNING
    session_id,
    current_stage,
    stage_direction,
    emotional_tone,
    interaction_mode,
    open_loop_summary,
    focus_topic_key,
    confidence,
    evidence_json,
    COALESCE(last_source_message_id, 0),
    created_at,
    updated_at
`

	var updated model.ConversationStateSlot
	err := s.pool.QueryRow(
		ctx,
		query,
		slot.SessionID,
		slot.CurrentStage,
		slot.StageDirection,
		slot.EmotionalTone,
		slot.InteractionMode,
		slot.OpenLoopSummary,
		slot.FocusTopicKey,
		slot.Confidence,
		slot.EvidenceJSON,
		slot.LastSourceMessageID,
	).Scan(
		&updated.SessionID,
		&updated.CurrentStage,
		&updated.StageDirection,
		&updated.EmotionalTone,
		&updated.InteractionMode,
		&updated.OpenLoopSummary,
		&updated.FocusTopicKey,
		&updated.Confidence,
		&updated.EvidenceJSON,
		&updated.LastSourceMessageID,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	return updated, err
}
