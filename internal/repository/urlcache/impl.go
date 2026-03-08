package urlcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

const (
	cacheTTL  = 24 * time.Hour
	keyPrefix = "shorturl:"
)

// Cache implements domain/repository.URLCache using Redis.
type Cache struct {
	rdb *redis.Client
}

// NewCache creates a new URL cache backed by the given Redis client.
func NewCache(rdb *redis.Client) *Cache {
	return &Cache{rdb: rdb}
}

// Get retrieves a ShortURL from cache by short code.
// Returns (nil, nil) on cache miss — not an error.
func (c *Cache) Get(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	data, err := c.rdb.Get(ctx, keyPrefix+shortCode).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // cache miss
		}
		return nil, fmt.Errorf("urlcache.Get: %w", err)
	}

	var url entity.ShortURL
	if err := json.Unmarshal(data, &url); err != nil {
		return nil, fmt.Errorf("urlcache.Get: unmarshal: %w", err)
	}
	return &url, nil
}

// Set stores a ShortURL in cache with a fixed TTL of 24 hours.
func (c *Cache) Set(ctx context.Context, shortCode string, url *entity.ShortURL) error {
	data, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("urlcache.Set: marshal: %w", err)
	}

	if err := c.rdb.Set(ctx, keyPrefix+shortCode, data, cacheTTL).Err(); err != nil {
		return fmt.Errorf("urlcache.Set: %w", err)
	}
	return nil
}

// Delete removes a ShortURL from cache by short code.
func (c *Cache) Delete(ctx context.Context, shortCode string) error {
	if err := c.rdb.Del(ctx, keyPrefix+shortCode).Err(); err != nil {
		return fmt.Errorf("urlcache.Delete: %w", err)
	}
	return nil
}
