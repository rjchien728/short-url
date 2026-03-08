package repository

import (
	"context"

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
type URLCache interface {
	Get(ctx context.Context, shortCode string) (*entity.ShortURL, error)
	Set(ctx context.Context, shortCode string, url *entity.ShortURL) error
	Delete(ctx context.Context, shortCode string) error
}

// EventPublisher publishes domain events to Redis Streams.
type EventPublisher interface {
	PublishClickEvent(ctx context.Context, event *entity.ClickLog) error
	PublishOGFetchTask(ctx context.Context, task *entity.OGFetchTask) error
}
