package postgres

import (
	"context"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type UpsertUserTraitParams struct {
	UserID          int64
	TraitType       string
	NormalizedValue string
	DisplayValue    string
	SourceText      string
}

func (s *Store) UpsertUserTrait(ctx context.Context, params UpsertUserTraitParams) (model.UserTrait, error) {
	const query = `
INSERT INTO tg_user_traits (
    user_id,
    trait_type,
    normalized_value,
    display_value,
    source_text,
    last_confirmed_at
) VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (user_id, normalized_value) DO UPDATE
SET
    trait_type = EXCLUDED.trait_type,
    display_value = EXCLUDED.display_value,
    source_text = EXCLUDED.source_text,
    last_confirmed_at = NOW(),
    updated_at = NOW()
RETURNING
    id,
    user_id,
    trait_type,
    normalized_value,
    display_value,
    COALESCE(source_text, ''),
    last_confirmed_at,
    created_at,
    updated_at
`

	var trait model.UserTrait
	err := s.pool.QueryRow(
		ctx,
		query,
		params.UserID,
		params.TraitType,
		params.NormalizedValue,
		params.DisplayValue,
		params.SourceText,
	).Scan(
		&trait.ID,
		&trait.UserID,
		&trait.TraitType,
		&trait.NormalizedValue,
		&trait.DisplayValue,
		&trait.SourceText,
		&trait.LastConfirmedAt,
		&trait.CreatedAt,
		&trait.UpdatedAt,
	)

	return trait, err
}

func (s *Store) ListUserTraitsByUserID(ctx context.Context, userID int64, limit int) ([]model.UserTrait, error) {
	if limit <= 0 {
		limit = 32
	}

	const query = `
SELECT
    id,
    user_id,
    trait_type,
    normalized_value,
    display_value,
    COALESCE(source_text, ''),
    last_confirmed_at,
    created_at,
    updated_at
FROM tg_user_traits
WHERE user_id = $1
ORDER BY updated_at DESC, id DESC
LIMIT $2
`

	rows, err := s.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	traits := make([]model.UserTrait, 0, limit)
	for rows.Next() {
		var trait model.UserTrait
		if err := rows.Scan(
			&trait.ID,
			&trait.UserID,
			&trait.TraitType,
			&trait.NormalizedValue,
			&trait.DisplayValue,
			&trait.SourceText,
			&trait.LastConfirmedAt,
			&trait.CreatedAt,
			&trait.UpdatedAt,
		); err != nil {
			return nil, err
		}
		traits = append(traits, trait)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return traits, nil
}
