package consumer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

const (
	ogStream    = "stream:og-fetch"
	clickStream = "stream:click-log"
	clickDLQ    = "stream:click-dlq"
)

const ogReadBlockTimeout = 5 * time.Second

// OGConsumer reads OG fetch tasks from stream:og-fetch and delegates to OGWorkerService.
// On either success or failure (FetchFailed already marked by the service), the message is
// always ACKed so it never blocks the queue — OG fetch failures are non-fatal.
type OGConsumer struct {
	rdb          *redis.Client
	ogService    service.OGWorkerService
	groupName    string
	consumer     string
	blockTimeout time.Duration // XReadGroup blocking duration
}

// NewOGConsumer creates an OGConsumer and ensures the consumer group exists.
// Panics if the group cannot be created (any error except BUSYGROUP).
func NewOGConsumer(rdb *redis.Client, svc service.OGWorkerService, groupName, consumerName string) *OGConsumer {
	err := rdb.XGroupCreateMkStream(context.Background(), ogStream, groupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		panic(fmt.Sprintf("og_consumer: failed to create consumer group: %v", err))
	}
	return &OGConsumer{
		rdb:          rdb,
		ogService:    svc,
		groupName:    groupName,
		consumer:     consumerName,
		blockTimeout: ogReadBlockTimeout,
	}
}

// WithBlockTimeout overrides the XReadGroup blocking duration.
// Intended for testing only.
func (c *OGConsumer) WithBlockTimeout(d time.Duration) *OGConsumer {
	c.blockTimeout = d
	return c
}

// Run starts the main read loop. It blocks until ctx is cancelled.
// On each iteration it reads one message, processes it, and always ACKs.
func (c *OGConsumer) Run(ctx context.Context) error {
	logger.Info(ctx, "og_consumer: started", "group", c.groupName, "consumer", c.consumer)
	for {
		// Check for cancellation before blocking.
		select {
		case <-ctx.Done():
			logger.Info(ctx, "og_consumer: stopped")
			return nil
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.groupName,
			Consumer: c.consumer,
			Streams:  []string{ogStream, ">"},
			Count:    1,
			Block:    c.blockTimeout,
		}).Result()
		if err != nil {
			// redis.Nil means blocking read timed out — normal, just retry.
			if err == redis.Nil {
				continue
			}
			// context cancelled while blocking — clean exit.
			if ctx.Err() != nil {
				logger.Info(ctx, "og_consumer: stopped")
				return nil
			}
			logger.Error(ctx, "og_consumer: XREADGROUP error", "error", err)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.processMessage(ctx, msg)
			}
		}
	}
}

// processMessage parses a Redis stream message and calls ogService.ProcessTask.
// The message is always ACKed afterwards regardless of the outcome.
func (c *OGConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	task, err := parseOGFetchTask(msg)
	if err != nil {
		logger.Error(ctx, "og_consumer: parse message failed, ACKing to avoid queue block",
			"msg_id", msg.ID, "error", err)
		c.ack(ctx, msg.ID)
		return
	}

	if err := c.ogService.ProcessTask(ctx, task); err != nil {
		logger.Error(ctx, "og_consumer: ProcessTask failed, ACKing (non-fatal)",
			"msg_id", msg.ID, "short_url_id", task.ShortURLID, "error", err)
	} else {
		logger.Info(ctx, "og_consumer: task processed",
			"msg_id", msg.ID, "short_url_id", task.ShortURLID, "long_url", task.LongURL)
	}

	// Always ACK — OG fetch failures are non-fatal and should not block the queue.
	c.ack(ctx, msg.ID)
}

// ack sends XACK for a single message; logs but does not return errors.
func (c *OGConsumer) ack(ctx context.Context, msgID string) {
	if err := c.rdb.XAck(ctx, ogStream, c.groupName, msgID).Err(); err != nil {
		logger.Error(ctx, "og_consumer: XACK failed", "msg_id", msgID, "error", err)
	}
}

// parseOGFetchTask converts a raw Redis stream message to an entity.OGFetchTask.
func parseOGFetchTask(msg redis.XMessage) (*entity.OGFetchTask, error) {
	shortURLIDStr, _ := msg.Values["short_url_id"].(string)
	longURL, _ := msg.Values["long_url"].(string)
	retryCountStr, _ := msg.Values["retry_count"].(string)

	shortURLID, err := strconv.ParseInt(shortURLIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse short_url_id %q: %w", shortURLIDStr, err)
	}
	retryCount, _ := strconv.Atoi(retryCountStr) // defaults to 0 on parse failure

	return &entity.OGFetchTask{
		ShortURLID: shortURLID,
		LongURL:    longURL,
		RetryCount: retryCount,
	}, nil
}
