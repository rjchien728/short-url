package og

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/consumer"
	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/pkg/logger"
	"github.com/rjchien728/short-url/internal/pkg/streamkey"
)

const defaultBlockTimeout = 5 * time.Second

// Consumer reads OG fetch tasks from stream:og-fetch and delegates to OGWorkerService.
// On either success or failure, the message is always ACKed so it never blocks the queue —
// OG fetch failures are non-fatal.
type Consumer struct {
	rdb          *redis.Client
	ogService    service.OGWorkerService
	groupName    string
	consumer     string
	blockTimeout time.Duration // XReadGroup blocking duration
}

// New creates a Consumer with the given parameters.
func New(rdb *redis.Client, svc service.OGWorkerService, groupName, consumerName string) *Consumer {
	return &Consumer{
		rdb:          rdb,
		ogService:    svc,
		groupName:    groupName,
		consumer:     consumerName,
		blockTimeout: defaultBlockTimeout,
	}
}

// WithBlockTimeout overrides the XReadGroup blocking duration.
// Intended for testing only.
func (c *Consumer) WithBlockTimeout(d time.Duration) *Consumer {
	c.blockTimeout = d
	return c
}

// Run starts the main read loop. It blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	return consumer.RunLoop(ctx, c.rdb, consumer.RunLoopOptions{
		Stream:       streamkey.OGFetch,
		Group:        c.groupName,
		Consumer:     c.consumer,
		Count:        1,
		BlockTimeout: c.blockTimeout,
		Processor:    c,
	})
}

// ProcessMessages implements consumer.MessageProcessor.
// It processes each message and always ACKs — OG fetch failures are non-fatal.
func (c *Consumer) ProcessMessages(ctx context.Context, rdb *redis.Client, msgs []redis.XMessage) error {
	for _, msg := range msgs {
		c.processMessage(ctx, rdb, msg)
	}
	return nil
}

// processMessage parses a Redis stream message and calls ogService.ProcessTask.
// The message is always ACKed afterwards regardless of the outcome.
func (c *Consumer) processMessage(ctx context.Context, rdb *redis.Client, msg redis.XMessage) {
	task, err := parseOGFetchTask(msg)
	if err != nil {
		logger.Error(ctx, "og_consumer: parse message failed, ACKing to avoid queue block",
			"msg_id", msg.ID, "error", err)
		c.ack(ctx, rdb, msg.ID)
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
	c.ack(ctx, rdb, msg.ID)
}

// ack sends XACK for a single message; logs but does not return errors.
func (c *Consumer) ack(ctx context.Context, rdb *redis.Client, msgID string) {
	if err := rdb.XAck(ctx, streamkey.OGFetch, c.groupName, msgID).Err(); err != nil {
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
