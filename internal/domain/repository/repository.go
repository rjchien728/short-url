//go:generate mockgen -destination=../../mock/mock_repository.go -package=mock github.com/rjchien728/short-url/internal/domain/repository ShortURLRepository,ClickLogRepository,URLCache,EventPublisher
package repository

import (
	"context"
	"time"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// ShortURLRepository provides persistence operations for ShortURL records.
type ShortURLRepository interface {
	Create(ctx context.Context, url *entity.ShortURL) error
	FindByShortCode(ctx context.Context, shortCode string) (*entity.ShortURL, error)
	UpdateOGMetadata(ctx context.Context, id int64, metadata *entity.OGMetadata) error
}

// ClickLogRepository provides persistence operations for ClickLog records.
type ClickLogRepository interface {
	BatchCreate(ctx context.Context, logs []*entity.ClickLog) error
}

// URLCache provides cache operations for ShortURL lookups.
// Cache miss is returned as (nil, nil) — not an error.
// Get returning a non-nil ErrNotFound sentinel signals a negative cache hit.
type URLCache interface {
	// Get retrieves a ShortURL from cache.
	// Returns (nil, nil) on cache miss.
	// Returns (nil, entity.ErrNotFound) when a negative-cache entry exists for shortCode.
	Get(ctx context.Context, shortCode string) (*entity.ShortURL, error)
	// Set stores a ShortURL in cache with the given TTL.
	Set(ctx context.Context, shortCode string, url *entity.ShortURL, ttl time.Duration) error
	// SetNotFound caches a negative entry (shortCode does not exist) with a short TTL.
	SetNotFound(ctx context.Context, shortCode string) error
	// Delete removes a ShortURL from cache.
	Delete(ctx context.Context, shortCode string) error
}

// EventPublisher publishes domain events to Redis Streams.
type EventPublisher interface {
	PublishClickEvent(ctx context.Context, event *entity.ClickLog) error
	PublishOGFetchTask(ctx context.Context, task *entity.OGFetchTask) error
}
