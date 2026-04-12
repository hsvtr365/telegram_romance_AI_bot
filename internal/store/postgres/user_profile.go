package postgres

import (
	"context"
	"errors"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/jackc/pgx/v5"
)

type UpsertUserProfileParams struct {
	UserID                     int64
	NameValue                  string
	GenderValue                string
	AgeValue                   string
	JobValue                   string
	CurrentFocusValue          string
	HobbyValue                 string
	LocationValue              string
	AffiliationValue           string
	LastRequestedSlot          string
	LastRequestedUserTurnCount int
	CollectionPausedUntilTurn  int
	PendingSlotsJSON           []byte
}

func (s *Store) GetUserProfileByUserID(ctx context.Context, userID int64) (model.UserProfile, error) {
	const query = `
SELECT
    user_id,
    COALESCE(name_value, ''),
    COALESCE(name_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(gender_value, ''),
    COALESCE(gender_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(age_value, ''),
    COALESCE(age_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(job_value, ''),
    COALESCE(job_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(current_focus_value, ''),
    COALESCE(current_focus_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(hobby_value, ''),
    COALESCE(hobby_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(location_value, ''),
    COALESCE(location_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(affiliation_value, ''),
    COALESCE(affiliation_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(last_requested_slot, ''),
    last_requested_user_turn_count,
    collection_paused_until_turn,
    COALESCE(pending_slots_jsonb, '{}'::jsonb),
    created_at,
    updated_at
FROM tg_user_profiles
WHERE user_id = $1
`

	var profile model.UserProfile
	err := s.pool.QueryRow(ctx, query, userID).Scan(
		&profile.UserID,
		&profile.NameValue,
		&profile.NameConfirmedAt,
		&profile.GenderValue,
		&profile.GenderConfirmedAt,
		&profile.AgeValue,
		&profile.AgeConfirmedAt,
		&profile.JobValue,
		&profile.JobConfirmedAt,
		&profile.CurrentFocusValue,
		&profile.CurrentFocusConfirmedAt,
		&profile.HobbyValue,
		&profile.HobbyConfirmedAt,
		&profile.LocationValue,
		&profile.LocationConfirmedAt,
		&profile.AffiliationValue,
		&profile.AffiliationConfirmedAt,
		&profile.LastRequestedSlot,
		&profile.LastRequestedUserTurnCount,
		&profile.CollectionPausedUntilTurn,
		&profile.PendingSlotsJSON,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.UserProfile{UserID: userID}, nil
	}
	return profile, err
}

func (s *Store) UpsertUserProfile(ctx context.Context, params UpsertUserProfileParams) (model.UserProfile, error) {
	const query = `
INSERT INTO tg_user_profiles (
    user_id,
    name_value,
    name_confirmed_at,
    gender_value,
    gender_confirmed_at,
    age_value,
    age_confirmed_at,
    job_value,
    job_confirmed_at,
    current_focus_value,
    current_focus_confirmed_at,
    hobby_value,
    hobby_confirmed_at,
    location_value,
    location_confirmed_at,
    affiliation_value,
    affiliation_confirmed_at,
    last_requested_slot,
    last_requested_user_turn_count,
    collection_paused_until_turn,
    pending_slots_jsonb
) VALUES (
    $1,
    NULLIF($2, ''),
    CASE WHEN NULLIF($2, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($3, ''),
    CASE WHEN NULLIF($3, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($4, ''),
    CASE WHEN NULLIF($4, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($5, ''),
    CASE WHEN NULLIF($5, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($6, ''),
    CASE WHEN NULLIF($6, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($7, ''),
    CASE WHEN NULLIF($7, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($8, ''),
    CASE WHEN NULLIF($8, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($9, ''),
    CASE WHEN NULLIF($9, '') IS NULL THEN NULL ELSE NOW() END,
    NULLIF($10, ''),
    GREATEST($11, 0),
    GREATEST($12, 0),
    COALESCE($13, '{}'::jsonb)
)
ON CONFLICT (user_id) DO UPDATE
SET
    name_value = COALESCE(NULLIF(EXCLUDED.name_value, ''), tg_user_profiles.name_value),
    name_confirmed_at = CASE WHEN NULLIF(EXCLUDED.name_value, '') IS NULL THEN tg_user_profiles.name_confirmed_at ELSE NOW() END,
    gender_value = COALESCE(NULLIF(EXCLUDED.gender_value, ''), tg_user_profiles.gender_value),
    gender_confirmed_at = CASE WHEN NULLIF(EXCLUDED.gender_value, '') IS NULL THEN tg_user_profiles.gender_confirmed_at ELSE NOW() END,
    age_value = COALESCE(NULLIF(EXCLUDED.age_value, ''), tg_user_profiles.age_value),
    age_confirmed_at = CASE WHEN NULLIF(EXCLUDED.age_value, '') IS NULL THEN tg_user_profiles.age_confirmed_at ELSE NOW() END,
    job_value = COALESCE(NULLIF(EXCLUDED.job_value, ''), tg_user_profiles.job_value),
    job_confirmed_at = CASE WHEN NULLIF(EXCLUDED.job_value, '') IS NULL THEN tg_user_profiles.job_confirmed_at ELSE NOW() END,
    current_focus_value = COALESCE(NULLIF(EXCLUDED.current_focus_value, ''), tg_user_profiles.current_focus_value),
    current_focus_confirmed_at = CASE WHEN NULLIF(EXCLUDED.current_focus_value, '') IS NULL THEN tg_user_profiles.current_focus_confirmed_at ELSE NOW() END,
    hobby_value = COALESCE(NULLIF(EXCLUDED.hobby_value, ''), tg_user_profiles.hobby_value),
    hobby_confirmed_at = CASE WHEN NULLIF(EXCLUDED.hobby_value, '') IS NULL THEN tg_user_profiles.hobby_confirmed_at ELSE NOW() END,
    location_value = COALESCE(NULLIF(EXCLUDED.location_value, ''), tg_user_profiles.location_value),
    location_confirmed_at = CASE WHEN NULLIF(EXCLUDED.location_value, '') IS NULL THEN tg_user_profiles.location_confirmed_at ELSE NOW() END,
    affiliation_value = COALESCE(NULLIF(EXCLUDED.affiliation_value, ''), tg_user_profiles.affiliation_value),
    affiliation_confirmed_at = CASE WHEN NULLIF(EXCLUDED.affiliation_value, '') IS NULL THEN tg_user_profiles.affiliation_confirmed_at ELSE NOW() END,
    last_requested_slot = COALESCE(NULLIF(EXCLUDED.last_requested_slot, ''), tg_user_profiles.last_requested_slot),
    last_requested_user_turn_count = CASE
        WHEN GREATEST(EXCLUDED.last_requested_user_turn_count, 0) = 0 THEN tg_user_profiles.last_requested_user_turn_count
        ELSE GREATEST(EXCLUDED.last_requested_user_turn_count, 0)
    END,
    collection_paused_until_turn = CASE
        WHEN GREATEST(EXCLUDED.collection_paused_until_turn, 0) = 0 THEN tg_user_profiles.collection_paused_until_turn
        ELSE GREATEST(EXCLUDED.collection_paused_until_turn, 0)
    END,
    pending_slots_jsonb = CASE
        WHEN $13 IS NULL THEN tg_user_profiles.pending_slots_jsonb
        ELSE COALESCE($13, '{}'::jsonb)
    END,
    updated_at = NOW()
RETURNING
    user_id,
    COALESCE(name_value, ''),
    COALESCE(name_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(gender_value, ''),
    COALESCE(gender_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(age_value, ''),
    COALESCE(age_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(job_value, ''),
    COALESCE(job_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(current_focus_value, ''),
    COALESCE(current_focus_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(hobby_value, ''),
    COALESCE(hobby_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(location_value, ''),
    COALESCE(location_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(affiliation_value, ''),
    COALESCE(affiliation_confirmed_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'),
    COALESCE(last_requested_slot, ''),
    last_requested_user_turn_count,
    collection_paused_until_turn,
    COALESCE(pending_slots_jsonb, '{}'::jsonb),
    created_at,
    updated_at
`

	var profile model.UserProfile
	err := s.pool.QueryRow(
		ctx,
		query,
		params.UserID,
		params.NameValue,
		params.GenderValue,
		params.AgeValue,
		params.JobValue,
		params.CurrentFocusValue,
		params.HobbyValue,
		params.LocationValue,
		params.AffiliationValue,
		params.LastRequestedSlot,
		params.LastRequestedUserTurnCount,
		params.CollectionPausedUntilTurn,
		params.PendingSlotsJSON,
	).Scan(
		&profile.UserID,
		&profile.NameValue,
		&profile.NameConfirmedAt,
		&profile.GenderValue,
		&profile.GenderConfirmedAt,
		&profile.AgeValue,
		&profile.AgeConfirmedAt,
		&profile.JobValue,
		&profile.JobConfirmedAt,
		&profile.CurrentFocusValue,
		&profile.CurrentFocusConfirmedAt,
		&profile.HobbyValue,
		&profile.HobbyConfirmedAt,
		&profile.LocationValue,
		&profile.LocationConfirmedAt,
		&profile.AffiliationValue,
		&profile.AffiliationConfirmedAt,
		&profile.LastRequestedSlot,
		&profile.LastRequestedUserTurnCount,
		&profile.CollectionPausedUntilTurn,
		&profile.PendingSlotsJSON,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	return profile, err
}

func (s *Store) MarkUserProfileSlotPrompted(ctx context.Context, userID int64, slot string, userTurnCount int) error {
	const query = `
INSERT INTO tg_user_profiles (
    user_id,
    last_requested_slot,
    last_requested_user_turn_count
) VALUES ($1, NULLIF($2, ''), GREATEST($3, 0))
ON CONFLICT (user_id) DO UPDATE
SET
    last_requested_slot = NULLIF(EXCLUDED.last_requested_slot, ''),
    last_requested_user_turn_count = GREATEST(EXCLUDED.last_requested_user_turn_count, 0),
    updated_at = NOW()
`

	_, err := s.pool.Exec(ctx, query, userID, slot, userTurnCount)
	return err
}

func (s *Store) PauseUserProfileCollection(ctx context.Context, userID int64, untilTurn int) error {
	const query = `
INSERT INTO tg_user_profiles (
    user_id,
    collection_paused_until_turn
) VALUES ($1, GREATEST($2, 0))
ON CONFLICT (user_id) DO UPDATE
SET
    collection_paused_until_turn = GREATEST(EXCLUDED.collection_paused_until_turn, 0),
    updated_at = NOW()
`

	_, err := s.pool.Exec(ctx, query, userID, untilTurn)
	return err
}

func (s *Store) CountUserTurns(ctx context.Context, sessionID int64) (int, error) {
	const query = `
SELECT COUNT(*)
FROM tg_chat_messages
WHERE session_id = $1
  AND role = 'user'
`

	var count int
	err := s.pool.QueryRow(ctx, query, sessionID).Scan(&count)
	return count, err
}
