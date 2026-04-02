package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitResult contains the outcome of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

type WindowLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
	Check(ctx context.Context, key string, limit int, window time.Duration) (*RateLimitResult, error)
}

type SlidingWindowLimiter struct {
	client *redis.Client
}

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

redis.call("ZREMRANGEBYSCORE", key, "-inf", now - window)
local count = redis.call("ZCARD", key)
if count >= limit then
  redis.call("EXPIRE", key, math.ceil(window / 1000))
  return {0, count, now + window}
end

redis.call("ZADD", key, now, member)
redis.call("EXPIRE", key, math.ceil(window / 1000))
return {1, count + 1, now + window}
`)

func NewSlidingWindowLimiter(client *redis.Client) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{client: client}
}

func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	result, err := l.Check(ctx, key, limit, window)
	if err != nil {
		return false, err
	}
	return result.Allowed, nil
}

func (l *SlidingWindowLimiter) Check(ctx context.Context, key string, limit int, window time.Duration) (*RateLimitResult, error) {
	nowMillis := time.Now().UnixMilli()
	member := fmt.Sprintf("%d-%d", nowMillis, time.Now().UnixNano())
	vals, err := slidingWindowScript.Run(ctx, l.client, []string{key}, nowMillis, window.Milliseconds(), limit, member).Int64Slice()
	if err != nil {
		return nil, err
	}
	allowed := vals[0] == 1
	count := int(vals[1])
	resetMillis := vals[2]

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimitResult{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   time.UnixMilli(resetMillis),
	}, nil
}
