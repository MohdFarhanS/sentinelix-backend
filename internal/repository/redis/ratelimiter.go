package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ingestRateLimit = 100
	ingestRateWindow = time.Minute
)

type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Fixed-window counter. Key pakai api_key_hash (bukan raw key) supaya raw
// key tidak bocor lewat Redis key name di log/MONITOR.
func (r *RateLimiter) Allow(ctx context.Context, apiKeyHash string) (bool, error) {
	window := time.Now().UTC().Format("200601021504")
	redisKey := fmt.Sprintf("ratelimit:%s:%s", apiKeyHash, window)

	count, err := r.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("ratelimiter incr: %w", err)	
	}
	if count == 1 {
		if err := r.client.Expire(ctx, redisKey, ingestRateWindow).Err(); err != nil {
			return false, fmt.Errorf("ratelimiter expire: %w", err)
		}
	}
	return count <= ingestRateLimit, nil
}