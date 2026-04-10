package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type ProactiveCooldown struct {
	LastProactiveAt time.Time `json:"last_proactive_at"`
	NextAllowedAt   time.Time `json:"next_allowed_at"`
	LastTriggerType string    `json:"last_trigger_type"`
}

type ProactivePresence struct {
	LastSeenAt time.Time `json:"last_seen_at"`
	Weekday    int       `json:"weekday"`
	Hour       int       `json:"hour"`
}

type ProactiveQueueItem struct {
	CandidateID string    `json:"candidate_id"`
	SessionID   int64     `json:"session_id"`
	DueAt       time.Time `json:"due_at"`
}

func (s *Store) AcquireProactiveLock(ctx context.Context, sessionID int64, workerID string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return s.client.SetNX(ctx, proactiveLockKey(sessionID), workerID, ttl).Result()
}

func (s *Store) ReleaseProactiveLock(ctx context.Context, sessionID int64, workerID string) (bool, error) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`

	raw, err := s.client.Eval(ctx, script, []string{proactiveLockKey(sessionID)}, workerID).Result()
	if err != nil {
		return false, err
	}

	switch value := raw.(type) {
	case int64:
		return value > 0, nil
	case int:
		return value > 0, nil
	default:
		return false, nil
	}
}

func (s *Store) SetProactiveCooldown(ctx context.Context, sessionID int64, cooldown ProactiveCooldown, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	payload, err := json.Marshal(cooldown)
	if err != nil {
		return err
	}

	key := proactiveCooldownKey(sessionID)
	if err := s.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) GetProactiveCooldown(ctx context.Context, sessionID int64) (ProactiveCooldown, bool, error) {
	value, err := s.client.Get(ctx, proactiveCooldownKey(sessionID)).Result()
	if err != nil {
		if err == goredis.Nil {
			return ProactiveCooldown{}, false, nil
		}
		return ProactiveCooldown{}, false, err
	}

	var cooldown ProactiveCooldown
	if err := json.Unmarshal([]byte(value), &cooldown); err != nil {
		return ProactiveCooldown{}, false, err
	}

	return cooldown, true, nil
}

func (s *Store) MarkProactiveTriggered(ctx context.Context, triggerType, refID string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}

	payload, err := json.Marshal(map[string]string{
		"trigger_type": triggerType,
		"ref_id":       refID,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return false, err
	}

	return s.client.SetNX(ctx, proactiveTriggeredKey(triggerType, refID), payload, ttl).Result()
}

func (s *Store) StoreProactiveCandidate(ctx context.Context, candidateID string, payload []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return s.client.Set(ctx, proactiveCandidateKey(candidateID), payload, ttl).Err()
}

func (s *Store) LoadProactiveCandidate(ctx context.Context, candidateID string) ([]byte, bool, error) {
	value, err := s.client.Get(ctx, proactiveCandidateKey(candidateID)).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}

	return value, true, nil
}

func (s *Store) EnqueueProactiveQueueItem(ctx context.Context, bucket string, item ProactiveQueueItem, ttl time.Duration) error {
	if bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	key := proactiveQueueKey(bucket)
	if err := s.client.ZAdd(ctx, key, goredis.Z{
		Score:  float64(item.DueAt.Unix()),
		Member: string(payload),
	}).Err(); err != nil {
		return err
	}

	return s.client.Expire(ctx, key, ttl).Err()
}

func (s *Store) ListDueProactiveQueueItems(ctx context.Context, bucket string, now time.Time, limit int) ([]ProactiveQueueItem, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if limit <= 0 {
		limit = 50
	}

	values, err := s.client.ZRangeByScore(ctx, proactiveQueueKey(bucket), &goredis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now.Unix()),
		Count: int64(limit),
	}).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, err
	}

	items := make([]ProactiveQueueItem, 0, len(values))
	for _, value := range values {
		var item ProactiveQueueItem
		if err := json.Unmarshal([]byte(value), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

func (s *Store) RemoveProactiveQueueItem(ctx context.Context, bucket string, item ProactiveQueueItem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return s.client.ZRem(ctx, proactiveQueueKey(bucket), string(payload)).Err()
}

func (s *Store) SetProactivePresence(ctx context.Context, sessionID int64, presence ProactivePresence, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}

	payload, err := json.Marshal(presence)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, proactivePresenceKey(sessionID), payload, ttl).Err()
}

func (s *Store) GetProactivePresence(ctx context.Context, sessionID int64) (ProactivePresence, bool, error) {
	value, err := s.client.Get(ctx, proactivePresenceKey(sessionID)).Result()
	if err != nil {
		if err == goredis.Nil {
			return ProactivePresence{}, false, nil
		}
		return ProactivePresence{}, false, err
	}

	var presence ProactivePresence
	if err := json.Unmarshal([]byte(value), &presence); err != nil {
		return ProactivePresence{}, false, err
	}

	return presence, true, nil
}

func (s *Store) DeleteProactiveSessionData(ctx context.Context, sessionIDs []int64) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(sessionIDs)*4)
	for _, sessionID := range sessionIDs {
		if sessionID == 0 {
			continue
		}
		keys = append(keys,
			proactiveLockKey(sessionID),
			proactiveCooldownKey(sessionID),
			proactivePresenceKey(sessionID),
			fmt.Sprintf("proactive:reply-pending:%d", sessionID),
		)
	}

	if len(keys) == 0 {
		return nil
	}

	return s.client.Del(ctx, keys...).Err()
}

func proactiveLockKey(sessionID int64) string {
	return fmt.Sprintf("proactive:lock:%d", sessionID)
}

func proactiveCooldownKey(sessionID int64) string {
	return fmt.Sprintf("proactive:cooldown:%d", sessionID)
}

func proactiveTriggeredKey(triggerType, refID string) string {
	return fmt.Sprintf("proactive:triggered:%s:%s", triggerType, refID)
}

func proactiveCandidateKey(candidateID string) string {
	return fmt.Sprintf("proactive:candidate:%s", candidateID)
}

func proactiveQueueKey(bucket string) string {
	return fmt.Sprintf("proactive:queue:%s", bucket)
}

func proactivePresenceKey(sessionID int64) string {
	return fmt.Sprintf("proactive:user-presence:%d", sessionID)
}
