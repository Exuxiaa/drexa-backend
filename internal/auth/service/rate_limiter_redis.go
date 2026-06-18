package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"drexa/internal/auth"
)

const rateLimitPrefix = "ratelimit:"

type redisRateLimiter struct {
	rdb *redis.Client
}

// NewRedisRateLimiter returns a RateLimiter backed by Redis using a fixed-window
// counter (INCR + EXPIRE), matching the pattern used by the OTP attempt tracker.
func NewRedisRateLimiter(rdb *redis.Client) auth.RateLimiter {
	return &redisRateLimiter{rdb: rdb}
}

func (l *redisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := rateLimitPrefix + key

	count, err := l.rdb.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("ratelimit: incr: %w", err)
	}

	// Set the expiry only on the first hit so the window starts at the first
	// attempt and does not slide forward on every subsequent request.
	if count == 1 {
		if err := l.rdb.Expire(ctx, fullKey, window).Err(); err != nil {
			return false, fmt.Errorf("ratelimit: expire: %w", err)
		}
	}

	return count <= int64(limit), nil
}
