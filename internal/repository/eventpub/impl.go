package eventpub

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

const (
	ogStream    = "stream:og-fetch"
	clickStream = "stream:click-log"
)

// Publisher implements domain/repository.EventPublisher using Redis Streams.
type Publisher struct {
	rdb *redis.Client
}

// NewPublisher creates a new event publisher backed by the given Redis client.
func NewPublisher(rdb *redis.Client) *Publisher {
	return &Publisher{rdb: rdb}
}

// PublishOGFetchTask sends an OG fetch task to the stream:og-fetch stream.
func (p *Publisher) PublishOGFetchTask(ctx context.Context, task *entity.OGFetchTask) error {
	err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: ogStream,
		ID:     "*",
		Values: map[string]any{
			"short_url_id": strconv.FormatInt(task.ShortURLID, 10),
			"long_url":     task.LongURL,
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("eventpub.PublishOGFetchTask: %w", err)
	}
	return nil
}

// PublishClickEvent sends a click log event to the stream:click-log stream.
func (p *Publisher) PublishClickEvent(ctx context.Context, event *entity.ClickLog) error {
	err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: clickStream,
		ID:     "*",
		Values: map[string]any{
			"id":           event.ID,
			"short_url_id": strconv.FormatInt(event.ShortURLID, 10),
			"short_code":   event.ShortCode,
			"creator_id":   event.CreatorID,
			"referral_id":  event.ReferralID,
			"referrer":     event.Referrer,
			"user_agent":   event.UserAgent,
			"ip_address":   event.IPAddress,
			"is_bot":       strconv.FormatBool(event.IsBot),
			"created_at":   event.CreatedAt.UTC().Format(time.RFC3339),
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("eventpub.PublishClickEvent: %w", err)
	}
	return nil
}
