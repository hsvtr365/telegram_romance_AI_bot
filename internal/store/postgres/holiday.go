package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ReplaceSpecialDaysForMonthKind(ctx context.Context, year int, month time.Month, kindCode string, items []model.SpecialDay) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const deleteQuery = `
DELETE FROM tg_special_days
WHERE EXTRACT(YEAR FROM day) = $1
  AND EXTRACT(MONTH FROM day) = $2
  AND kind_code = $3
`
	if _, err := tx.Exec(ctx, deleteQuery, year, int(month), kindCode); err != nil {
		return err
	}

	const insertQuery = `
INSERT INTO tg_special_days (
    day,
    name,
    kind_code,
    kind_label,
    is_holiday,
    seq,
    is_major_holiday,
    major_holiday_group,
    source_payload,
    fetched_at
) VALUES ($1::date, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), COALESCE($9::jsonb, '{}'::jsonb), $10)
ON CONFLICT (day, kind_code, seq, name) DO UPDATE
SET
    kind_label = EXCLUDED.kind_label,
    is_holiday = EXCLUDED.is_holiday,
    is_major_holiday = EXCLUDED.is_major_holiday,
    major_holiday_group = EXCLUDED.major_holiday_group,
    source_payload = EXCLUDED.source_payload,
    fetched_at = EXCLUDED.fetched_at,
    updated_at = NOW()
`

	for _, item := range items {
		day := normalizeDate(item.Day)
		if day.IsZero() {
			return fmt.Errorf("special day has zero day")
		}
		if _, err := tx.Exec(
			ctx,
			insertQuery,
			day.Format("2006-01-02"),
			item.Name,
			item.KindCode,
			item.KindLabel,
			item.IsHoliday,
			item.Seq,
			item.IsMajorHoliday,
			item.MajorHolidayGroup,
			item.SourcePayload,
			nullTime(item.FetchedAt),
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) ListSpecialDays(ctx context.Context, from time.Time, to time.Time) ([]model.SpecialDay, error) {
	const query = `
SELECT
    id,
    day,
    name,
    kind_code,
    kind_label,
    is_holiday,
    seq,
    is_major_holiday,
    COALESCE(major_holiday_group, ''),
    COALESCE(source_payload, '{}'::jsonb),
    fetched_at,
    created_at,
    updated_at
FROM tg_special_days
WHERE day BETWEEN $1::date AND $2::date
ORDER BY day ASC, is_holiday DESC, is_major_holiday DESC, seq ASC, name ASC
`

	rows, err := s.pool.Query(ctx, query, normalizeDate(from).Format("2006-01-02"), normalizeDate(to).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.SpecialDay, 0, 16)
	for rows.Next() {
		var item model.SpecialDay
		if err := rows.Scan(
			&item.ID,
			&item.Day,
			&item.Name,
			&item.KindCode,
			&item.KindLabel,
			&item.IsHoliday,
			&item.Seq,
			&item.IsMajorHoliday,
			&item.MajorHolidayGroup,
			&item.SourcePayload,
			&item.FetchedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *Store) HasSessionPromptTopic(ctx context.Context, sessionID int64, topicKey string) (bool, error) {
	const query = `
SELECT 1
FROM tg_session_prompt_topics
WHERE session_id = $1
  AND topic_key = $2
LIMIT 1
`

	var exists int
	err := s.pool.QueryRow(ctx, query, sessionID, topicKey).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (s *Store) MarkSessionPromptTopicUsed(ctx context.Context, topic model.SessionPromptTopic) error {
	const query = `
INSERT INTO tg_session_prompt_topics (
    session_id,
    topic_key,
    topic_type,
    topic_date,
    source,
    used_at
) VALUES ($1, $2, $3, $4::date, $5, $6)
ON CONFLICT (session_id, topic_key) DO NOTHING
`

	_, err := s.pool.Exec(
		ctx,
		query,
		topic.SessionID,
		topic.TopicKey,
		topic.TopicType,
		normalizeDate(topic.TopicDate).Format("2006-01-02"),
		topic.Source,
		nullTime(topic.UsedAt),
	)
	return err
}

func normalizeDate(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}
