package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const recentTTL = 7 * 24 * time.Hour

type Turn struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	SavedAt time.Time `json:"saved_at"`
}

type Store struct {
	client *goredis.Client
}

func New(ctx context.Context, redisURL string) (*Store, error) {
	options, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := goredis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *Store) AppendRecent(ctx context.Context, sessionID int64, turn Turn, limit int) error {
	payload, err := json.Marshal(turn)
	if err != nil {
		return err
	}

	key := recentKey(sessionID)
	pipe := s.client.TxPipeline()
	pipe.RPush(ctx, key, payload)
	pipe.LTrim(ctx, key, int64(-limit), -1)
	pipe.Expire(ctx, key, recentTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) GetRecent(ctx context.Context, sessionID int64) ([]Turn, error) {
	key := recentKey(sessionID)
	values, err := s.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, err
	}

	turns := make([]Turn, 0, len(values))
	for _, value := range values {
		var turn Turn
		if err := json.Unmarshal([]byte(value), &turn); err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}

	return turns, nil
}

func (s *Store) DeleteRecent(ctx context.Context, sessionIDs []int64) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if sessionID == 0 {
			continue
		}
		keys = append(keys, recentKey(sessionID))
	}

	if len(keys) == 0 {
		return nil
	}

	return s.client.Del(ctx, keys...).Err()
}

func recentKey(sessionID int64) string {
	return fmt.Sprintf("chat:recent:%d", sessionID)
}
