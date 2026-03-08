package infra

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and returns a *redis.Client based on RedisConfig.
// URL format: redis://[:password@]host[:port][/db-number]
// Example: Cache: redis://localhost:6379/0, Stream: redis://localhost:6379/1
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*redis.Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("redis URL is required")
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Validate connection
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
