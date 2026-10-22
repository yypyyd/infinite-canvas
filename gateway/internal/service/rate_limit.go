package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRateLimited = errors.New("rate limited")

type RateLimitService struct {
	redis  *redis.Client
	prefix string
}

type RateLimitResult struct {
	Allowed    bool
	Count      int64
	Limit      int64
	RetryAfter time.Duration
}

func NewRateLimitService(redis *redis.Client) *RateLimitService {
	return &RateLimitService{
		redis:  redis,
		prefix: "rl:",
	}
}

func (s *RateLimitService) Allow(ctx context.Context, bucket string, limit int64, window time.Duration) (*RateLimitResult, error) {
	if limit <= 0 || window <= 0 {
		return &RateLimitResult{Allowed: true, Limit: limit}, nil
	}

	key := s.prefix + strings.TrimSpace(bucket)
	count, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		if err := s.redis.Expire(ctx, key, window).Err(); err != nil {
			return nil, err
		}
	}

	ttl, err := s.redis.TTL(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if ttl < 0 {
		ttl = window
	}

	return &RateLimitResult{
		Allowed:    count <= limit,
		Count:      count,
		Limit:      limit,
		RetryAfter: ttl,
	}, nil
}

func (s *RateLimitService) Enforce(ctx context.Context, bucket string, limit int64, window time.Duration) error {
	result, err := s.Allow(ctx, bucket, limit, window)
	if err != nil {
		return err
	}
	if result.Allowed {
		return nil
	}
	retry := int(result.RetryAfter.Seconds())
	if retry < 1 {
		retry = 1
	}
	return fmt.Errorf("%w: 请稍后再试（%d 秒后）", ErrRateLimited, retry)
}
