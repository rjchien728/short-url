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
	// CacheTTL is the base TTL for a cached short URL.
	// Every cache hit resets the TTL (sliding window), so hot URLs stay cached
	// indefinitely while cold URLs expire after one idle period.
	CacheTTL = 1 * time.Hour

	// negativeTTL is the TTL for negative-cache entries (shortCode not found in DB).
	// Short enough that a newly created URL becomes visible quickly.
	negativeTTL = 1 * time.Minute

	// negativeMarker is the sentinel value stored for a negative cache entry.
	// It must not be valid JSON for a ShortURL, so a plain string works.
	negativeMarker = "NOT_FOUND"

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
//   - Cache miss  → (nil, nil)
//   - Negative hit → (nil, entity.ErrNotFound)
//   - Normal hit  → (*ShortURL, nil), and the TTL is refreshed (sliding window)
func (c *Cache) Get(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	key := keyPrefix + shortCode

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // cache miss
		}
		return nil, fmt.Errorf("urlcache.Get: %w", err)
	}

	// Negative cache sentinel — shortCode is known to not exist.
	if string(data) == negativeMarker {
		return nil, entity.ErrNotFound
	}

	var url entity.ShortURL
	if err := json.Unmarshal(data, &url); err != nil {
		return nil, fmt.Errorf("urlcache.Get: unmarshal: %w", err)
	}

	// Sliding window: reset TTL on every hit so hot URLs stay in cache.
	// Fire-and-forget; a failed Expire is non-fatal (key still valid until old TTL).
	_ = c.rdb.Expire(ctx, key, CacheTTL).Err()

	return &url, nil
}

// Set stores a ShortURL in cache with the provided TTL.
func (c *Cache) Set(ctx context.Context, shortCode string, url *entity.ShortURL, ttl time.Duration) error {
	data, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("urlcache.Set: marshal: %w", err)
	}

	if err := c.rdb.Set(ctx, keyPrefix+shortCode, data, ttl).Err(); err != nil {
		return fmt.Errorf("urlcache.Set: %w", err)
	}
	return nil
}

// SetNotFound caches a negative entry for shortCode with a short TTL.
// Subsequent Get calls for this code will return entity.ErrNotFound without hitting the DB.
func (c *Cache) SetNotFound(ctx context.Context, shortCode string) error {
	if err := c.rdb.Set(ctx, keyPrefix+shortCode, negativeMarker, negativeTTL).Err(); err != nil {
		return fmt.Errorf("urlcache.SetNotFound: %w", err)
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
