package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/jackc/pgx/v5"
)

const profileCandidateEvidenceLimit = 5

type MergeProfileCandidateParams struct {
	UserID           int64
	SlotName         string
	CandidateValue   string
	NormalizedValue  string
	BestEvidenceType string
	BestConfidence   string
	Evidence         model.ProfileCandidateEvidence
}

type UpdateProfileCandidateStatusParams struct {
	UserID          int64
	SlotName        string
	NormalizedValue string
	Status          string
	ReviewedAt      time.Time
	PromotedAt      time.Time
}

func (s *Store) MergeProfileCandidate(ctx context.Context, params MergeProfileCandidateParams) (model.ProfileCandidate, error) {
	params.SlotName = strings.TrimSpace(params.SlotName)
	params.CandidateValue = strings.TrimSpace(params.CandidateValue)
	params.NormalizedValue = strings.TrimSpace(params.NormalizedValue)
	params.BestEvidenceType = normalizeProfileCandidateEvidenceType(params.BestEvidenceType)
	params.BestConfidence = normalizeProfileCandidateConfidence(params.BestConfidence)
	params.Evidence.EvidenceText = strings.TrimSpace(params.Evidence.EvidenceText)
	params.Evidence.EvidenceType = normalizeProfileCandidateEvidenceType(params.Evidence.EvidenceType)
	params.Evidence.Confidence = normalizeProfileCandidateConfidence(params.Evidence.Confidence)
	if params.Evidence.CapturedAt.IsZero() {
		params.Evidence.CapturedAt = time.Now()
	}
	if params.UserID == 0 || params.SlotName == "" || params.CandidateValue == "" || params.NormalizedValue == "" {
		return model.ProfileCandidate{}, errors.New("invalid profile candidate params")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ProfileCandidate{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	existing, found, err := getProfileCandidateForUpdate(ctx, tx, params.UserID, params.SlotName, params.NormalizedValue)
	if err != nil {
		return model.ProfileCandidate{}, err
	}

	now := time.Now()
	if !found {
		evidenceJSON, err := json.Marshal(trimProfileCandidateEvidence([]model.ProfileCandidateEvidence{params.Evidence}))
		if err != nil {
			return model.ProfileCandidate{}, err
		}

		const insertQuery = `
INSERT INTO tg_profile_candidates (
    user_id,
    slot_name,
    candidate_value,
    normalized_value,
    best_evidence_type,
    best_confidence,
    evidence_messages_jsonb,
    mention_count,
    first_seen_at,
    last_seen_at,
    status,
    reviewed_at,
    promoted_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 1, $8, $8, $9, NULL, NULL
)
RETURNING
    id,
    user_id,
    slot_name,
    candidate_value,
    normalized_value,
    best_evidence_type,
    best_confidence,
    evidence_messages_jsonb,
    mention_count,
    first_seen_at,
    last_seen_at,
    status,
    COALESCE(reviewed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(promoted_at, TIMESTAMPTZ '0001-01-01 00:00:00+00')
`

		candidate, err := scanProfileCandidateRow(
			tx.QueryRow(
				ctx,
				insertQuery,
				params.UserID,
				params.SlotName,
				params.CandidateValue,
				params.NormalizedValue,
				params.BestEvidenceType,
				params.BestConfidence,
				evidenceJSON,
				now,
				model.ProfileCandidateStatusActive,
			),
		)
		if err != nil {
			return model.ProfileCandidate{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.ProfileCandidate{}, err
		}
		return candidate, nil
	}

	existing.CandidateValue = coalesceNonEmpty(params.CandidateValue, existing.CandidateValue)
	existing.BestEvidenceType = strongerProfileCandidateEvidenceType(existing.BestEvidenceType, params.BestEvidenceType)
	existing.BestConfidence = strongerProfileCandidateConfidence(existing.BestConfidence, params.BestConfidence)
	existing.EvidenceMessages = mergeProfileCandidateEvidence(existing.EvidenceMessages, params.Evidence)
	existing.MentionCount++
	existing.LastSeenAt = now
	if existing.Status == model.ProfileCandidateStatusDismissed {
		existing.Status = model.ProfileCandidateStatusActive
	}
	existing.ReviewedAt = time.Time{}

	evidenceJSON, err := json.Marshal(existing.EvidenceMessages)
	if err != nil {
		return model.ProfileCandidate{}, err
	}

	const updateQuery = `
UPDATE tg_profile_candidates
SET
    candidate_value = $4,
    best_evidence_type = $5,
    best_confidence = $6,
    evidence_messages_jsonb = $7,
    mention_count = $8,
    last_seen_at = $9,
    status = $10,
    reviewed_at = NULL,
    updated_at = NOW()
WHERE user_id = $1 AND slot_name = $2 AND normalized_value = $3
RETURNING
    id,
    user_id,
    slot_name,
    candidate_value,
    normalized_value,
    best_evidence_type,
    best_confidence,
    evidence_messages_jsonb,
    mention_count,
    first_seen_at,
    last_seen_at,
    status,
    COALESCE(reviewed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(promoted_at, TIMESTAMPTZ '0001-01-01 00:00:00+00')
`

	candidate, err := scanProfileCandidateRow(
		tx.QueryRow(
			ctx,
			updateQuery,
			params.UserID,
			params.SlotName,
			params.NormalizedValue,
			existing.CandidateValue,
			existing.BestEvidenceType,
			existing.BestConfidence,
			evidenceJSON,
			existing.MentionCount,
			existing.LastSeenAt,
			existing.Status,
		),
	)
	if err != nil {
		return model.ProfileCandidate{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.ProfileCandidate{}, err
	}

	return candidate, nil
}

func (s *Store) ListTopProfileCandidatesByUserID(ctx context.Context, userID int64) ([]model.ProfileCandidate, error) {
	const query = `
SELECT DISTINCT ON (slot_name)
    id,
    user_id,
    slot_name,
    candidate_value,
    normalized_value,
    best_evidence_type,
    best_confidence,
    evidence_messages_jsonb,
    mention_count,
    first_seen_at,
    last_seen_at,
    status,
    COALESCE(reviewed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(promoted_at, TIMESTAMPTZ '0001-01-01 00:00:00+00')
FROM tg_profile_candidates
WHERE user_id = $1
  AND status = 'active'
ORDER BY
    slot_name ASC,
    CASE best_evidence_type
        WHEN 'explicit' THEN 2
        WHEN 'tentative' THEN 1
        ELSE 0
    END DESC,
    mention_count DESC,
    last_seen_at DESC,
    id DESC
`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]model.ProfileCandidate, 0, 8)
	for rows.Next() {
		candidate, err := scanProfileCandidateRow(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return candidates, nil
}

func (s *Store) UpdateProfileCandidateStatus(ctx context.Context, params UpdateProfileCandidateStatusParams) error {
	params.Status = normalizeProfileCandidateStatus(params.Status)
	if params.UserID == 0 || strings.TrimSpace(params.SlotName) == "" || strings.TrimSpace(params.NormalizedValue) == "" {
		return nil
	}

	const query = `
UPDATE tg_profile_candidates
SET
    status = $4,
    reviewed_at = CASE
        WHEN $5 = TIMESTAMPTZ '0001-01-01 00:00:00+00' THEN reviewed_at
        ELSE $5
    END,
    promoted_at = CASE
        WHEN $6 = TIMESTAMPTZ '0001-01-01 00:00:00+00' THEN promoted_at
        ELSE $6
    END,
    updated_at = NOW()
WHERE user_id = $1
  AND slot_name = $2
  AND normalized_value = $3
`

	_, err := s.pool.Exec(
		ctx,
		query,
		params.UserID,
		strings.TrimSpace(params.SlotName),
		strings.TrimSpace(params.NormalizedValue),
		params.Status,
		zeroTimeIfEmpty(params.ReviewedAt),
		zeroTimeIfEmpty(params.PromotedAt),
	)
	return err
}

func (s *Store) ListProfileReviewWindowMessages(ctx context.Context, sessionID int64, userTurnLimit int) ([]model.Message, error) {
	if sessionID == 0 || userTurnLimit <= 0 {
		return nil, nil
	}

	const query = `
WITH recent_user_turns AS (
    SELECT id
    FROM tg_chat_messages
    WHERE session_id = $1
      AND role = 'user'
    ORDER BY id DESC
    LIMIT $2
),
window_start AS (
    SELECT COALESCE(MIN(id), 0) AS start_id
    FROM recent_user_turns
)
SELECT id, session_id, role, content, mode, created_at
FROM tg_chat_messages
WHERE session_id = $1
  AND id >= (SELECT start_id FROM window_start)
ORDER BY id ASC
`

	rows, err := s.pool.Query(ctx, query, sessionID, userTurnLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]model.Message, 0, userTurnLimit*2)
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

func getProfileCandidateForUpdate(ctx context.Context, tx pgx.Tx, userID int64, slotName string, normalizedValue string) (model.ProfileCandidate, bool, error) {
	const query = `
SELECT
    id,
    user_id,
    slot_name,
    candidate_value,
    normalized_value,
    best_evidence_type,
    best_confidence,
    evidence_messages_jsonb,
    mention_count,
    first_seen_at,
    last_seen_at,
    status,
    COALESCE(reviewed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(promoted_at, TIMESTAMPTZ '0001-01-01 00:00:00+00')
FROM tg_profile_candidates
WHERE user_id = $1
  AND slot_name = $2
  AND normalized_value = $3
FOR UPDATE
`

	candidate, err := scanProfileCandidateRow(tx.QueryRow(ctx, query, userID, slotName, normalizedValue))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ProfileCandidate{}, false, nil
	}
	if err != nil {
		return model.ProfileCandidate{}, false, err
	}
	return candidate, true, nil
}

type profileCandidateRowScanner interface {
	Scan(dest ...any) error
}

func scanProfileCandidateRow(scanner profileCandidateRowScanner) (model.ProfileCandidate, error) {
	var (
		candidate    model.ProfileCandidate
		evidenceJSON []byte
	)

	err := scanner.Scan(
		&candidate.ID,
		&candidate.UserID,
		&candidate.SlotName,
		&candidate.CandidateValue,
		&candidate.NormalizedValue,
		&candidate.BestEvidenceType,
		&candidate.BestConfidence,
		&evidenceJSON,
		&candidate.MentionCount,
		&candidate.FirstSeenAt,
		&candidate.LastSeenAt,
		&candidate.Status,
		&candidate.ReviewedAt,
		&candidate.PromotedAt,
	)
	if err != nil {
		return model.ProfileCandidate{}, err
	}

	candidate.BestEvidenceType = normalizeProfileCandidateEvidenceType(candidate.BestEvidenceType)
	candidate.BestConfidence = normalizeProfileCandidateConfidence(candidate.BestConfidence)
	candidate.Status = normalizeProfileCandidateStatus(candidate.Status)
	if len(evidenceJSON) > 0 {
		_ = json.Unmarshal(evidenceJSON, &candidate.EvidenceMessages)
		candidate.EvidenceMessages = trimProfileCandidateEvidence(candidate.EvidenceMessages)
	}

	return candidate, nil
}

func mergeProfileCandidateEvidence(existing []model.ProfileCandidateEvidence, next model.ProfileCandidateEvidence) []model.ProfileCandidateEvidence {
	merged := make([]model.ProfileCandidateEvidence, 0, len(existing)+1)
	seen := make(map[string]struct{}, len(existing)+1)

	push := func(item model.ProfileCandidateEvidence) {
		item.EvidenceText = strings.TrimSpace(item.EvidenceText)
		item.EvidenceType = normalizeProfileCandidateEvidenceType(item.EvidenceType)
		item.Confidence = normalizeProfileCandidateConfidence(item.Confidence)
		if item.CapturedAt.IsZero() {
			item.CapturedAt = time.Now()
		}
		key := strings.Join([]string{
			item.EvidenceType,
			item.Confidence,
			strings.TrimSpace(item.EvidenceText),
			strconv.FormatInt(item.MessageID, 10),
		}, "|")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, item)
	}

	push(next)
	for _, item := range existing {
		push(item)
	}

	return trimProfileCandidateEvidence(merged)
}

func trimProfileCandidateEvidence(values []model.ProfileCandidateEvidence) []model.ProfileCandidateEvidence {
	filtered := make([]model.ProfileCandidateEvidence, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.EvidenceText) == "" && value.MessageID == 0 {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) > profileCandidateEvidenceLimit {
		filtered = filtered[:profileCandidateEvidenceLimit]
	}
	return filtered
}

func strongerProfileCandidateEvidenceType(current string, next string) string {
	if profileCandidateEvidenceRank(next) > profileCandidateEvidenceRank(current) {
		return normalizeProfileCandidateEvidenceType(next)
	}
	return normalizeProfileCandidateEvidenceType(current)
}

func profileCandidateEvidenceRank(value string) int {
	switch normalizeProfileCandidateEvidenceType(value) {
	case "explicit":
		return 2
	case "tentative":
		return 1
	default:
		return 0
	}
}

func strongerProfileCandidateConfidence(current string, next string) string {
	if profileCandidateConfidenceRank(next) > profileCandidateConfidenceRank(current) {
		return normalizeProfileCandidateConfidence(next)
	}
	return normalizeProfileCandidateConfidence(current)
}

func profileCandidateConfidenceRank(value string) int {
	switch normalizeProfileCandidateConfidence(value) {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func normalizeProfileCandidateEvidenceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "explicit":
		return "explicit"
	case "tentative":
		return "tentative"
	default:
		return "none"
	}
}

func normalizeProfileCandidateConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func normalizeProfileCandidateStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.ProfileCandidateStatusConfirmed:
		return model.ProfileCandidateStatusConfirmed
	case model.ProfileCandidateStatusDismissed:
		return model.ProfileCandidateStatusDismissed
	default:
		return model.ProfileCandidateStatusActive
	}
}

func coalesceNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func zeroTimeIfEmpty(value time.Time) time.Time {
	if value.IsZero() {
		return time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return value
}
