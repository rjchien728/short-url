package infra

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient 依照 RedisConfig 建立並回傳一個 *redis.Client。
// URL 格式：redis://[:password@]host[:port][/db-number]
// 例如 Cache: redis://localhost:6379/0，Stream: redis://localhost:6379/1
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*redis.Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("redis URL is required")
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// 驗證連線是否可用
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
