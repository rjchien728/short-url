package click

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

const (
	// defaultReclaimInterval is how often the reclaim goroutine runs.
	defaultReclaimInterval = 10 * time.Second

	// defaultIdleThreshold is the minimum idle time before a PEL message is reclaimed.
	defaultIdleThreshold = 30 * time.Second

	// defaultBlockTimeout is the XReadGroup blocking duration.
	defaultBlockTimeout = 5 * time.Second
)

// Consumer reads click log events from stream:click-log in batches and
// delegates to ClickWorkerService. Failed batches are not ACKed so they stay
// in the PEL for retry via XCLAIM. Messages that exceed maxDelivery are moved
// to stream:click-dlq (dead letter queue) and then ACKed.
type Consumer struct {
	rdb             *redis.Client
	clickService    service.ClickWorkerService
	groupName       string
	consumer        string
	batchSize       int64
	maxDelivery     int64
	reclaimInterval time.Duration // how often reclaimLoop runs
	idleThreshold   time.Duration // min idle time before a PEL message is reclaimed
	blockTimeout    time.Duration // XReadGroup blocking duration
}

// New creates a Consumer with the given parameters.
func New(rdb *redis.Client, svc service.ClickWorkerService, groupName, consumerName string, batchSize, maxDelivery int) *Consumer {
	return &Consumer{
		rdb:             rdb,
		clickService:    svc,
		groupName:       groupName,
		consumer:        consumerName,
		batchSize:       int64(batchSize),
		maxDelivery:     int64(maxDelivery),
		reclaimInterval: defaultReclaimInterval,
		idleThreshold:   defaultIdleThreshold,
		blockTimeout:    defaultBlockTimeout,
	}
}

// WithReclaimInterval overrides the reclaim loop interval.
// Intended for testing only.
func (c *Consumer) WithReclaimInterval(d time.Duration) *Consumer {
	c.reclaimInterval = d
	return c
}

// WithIdleThreshold overrides the PEL idle threshold used in XPendingExt and XClaim.
// Intended for testing only.
func (c *Consumer) WithIdleThreshold(d time.Duration) *Consumer {
	c.idleThreshold = d
	return c
}

// WithBlockTimeout overrides the XReadGroup blocking duration.
// Intended for testing only.
func (c *Consumer) WithBlockTimeout(d time.Duration) *Consumer {
	c.blockTimeout = d
	return c
}

// Run starts the main read loop and the reclaim goroutine concurrently.
// Both exit when ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	go c.reclaimLoop(ctx)

	return consumer.RunLoop(ctx, c.rdb, consumer.RunLoopOptions{
		Stream:       streamkey.ClickLog,
		Group:        c.groupName,
		Consumer:     c.consumer,
		Count:        c.batchSize,
		BlockTimeout: c.blockTimeout,
		Processor:    c,
	})
}

// ProcessMessages implements consumer.MessageProcessor.
// It parses messages, calls ProcessBatch, and ACKs only on success.
// On failure the messages stay in the PEL for retry via XCLAIM.
func (c *Consumer) ProcessMessages(ctx context.Context, rdb *redis.Client, msgs []redis.XMessage) error {
	// Parse messages and collect valid logs with their IDs in a single pass.
	// logs and ids are strictly 1:1 — malformed messages are ACKed immediately and skipped.
	logs := make([]*entity.ClickLog, 0, len(msgs))
	ids := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		log, err := parseClickLog(msg)
		if err != nil {
			// Malformed message: ACK immediately to avoid infinite retry.
			logger.Error(ctx, "click_consumer: parse failed, ACKing malformed message",
				"msg_id", msg.ID, "error", err)
			_ = rdb.XAck(ctx, streamkey.ClickLog, c.groupName, msg.ID).Err()
			continue
		}
		logs = append(logs, log)
		ids = append(ids, msg.ID)
	}

	if len(logs) == 0 {
		return nil
	}

	if err := c.clickService.ProcessBatch(ctx, logs); err != nil {
		// Do NOT ACK — messages stay in PEL and will be retried via XCLAIM.
		logger.Error(ctx, "click_consumer: ProcessBatch failed, messages left in PEL",
			"count", len(logs), "error", err)
		return nil
	}

	// Success — ACK all messages in the batch.
	if err := rdb.XAck(ctx, streamkey.ClickLog, c.groupName, ids...).Err(); err != nil {
		logger.Error(ctx, "click_consumer: XACK failed after successful batch",
			"count", len(ids), "error", err)
		return nil
	}
	logger.Info(ctx, "click_consumer: batch processed", "count", len(logs))
	return nil
}

// reclaimLoop runs on a fixed interval to reclaim idle PEL messages and move
// messages that exceeded maxDelivery to the dead letter queue.
func (c *Consumer) reclaimLoop(ctx context.Context) {
	ticker := time.NewTicker(c.reclaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reclaim(ctx)
		}
	}
}

// reclaim queries the PEL for idle messages and either re-claims them for
// retry or moves them to the DLQ if delivery count exceeds maxDelivery.
func (c *Consumer) reclaim(ctx context.Context) {
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamkey.ClickLog,
		Group:  c.groupName,
		Start:  "-",
		End:    "+",
		Count:  c.batchSize,
		Idle:   c.idleThreshold,
	}).Result()
	if err != nil {
		logger.Error(ctx, "click_consumer: XPendingExt error", "error", err)
		return
	}

	for _, p := range pending {
		if p.RetryCount > c.maxDelivery {
			// Too many delivery attempts — send to DLQ and ACK original.
			c.sendToDLQ(ctx, p.ID)
			continue
		}

		// Reclaim the message so this consumer can retry it.
		if err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream:   streamkey.ClickLog,
			Group:    c.groupName,
			Consumer: c.consumer,
			MinIdle:  c.idleThreshold,
			Messages: []string{p.ID},
		}).Err(); err != nil {
			logger.Error(ctx, "click_consumer: XClaim failed", "msg_id", p.ID, "error", err)
		}
	}
}

// sendToDLQ moves a message to stream:click-dlq, then ACKs it from the main stream.
func (c *Consumer) sendToDLQ(ctx context.Context, msgID string) {
	// Read the original message to copy its payload to the DLQ.
	msgs, err := c.rdb.XRange(ctx, streamkey.ClickLog, msgID, msgID).Result()
	if err != nil || len(msgs) == 0 {
		logger.Error(ctx, "click_consumer: cannot read message for DLQ transfer",
			"msg_id", msgID, "error", err)
		// ACK anyway to avoid an infinite loop.
		_ = c.rdb.XAck(ctx, streamkey.ClickLog, c.groupName, msgID).Err()
		return
	}

	// Write to DLQ with original payload preserved.
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamkey.ClickDLQ,
		ID:     "*",
		Values: msgs[0].Values,
	}).Err(); err != nil {
		logger.Error(ctx, "click_consumer: failed to write to DLQ", "msg_id", msgID, "error", err)
	}

	// ACK the original to remove it from PEL.
	if err := c.rdb.XAck(ctx, streamkey.ClickLog, c.groupName, msgID).Err(); err != nil {
		logger.Error(ctx, "click_consumer: XACK after DLQ failed", "msg_id", msgID, "error", err)
	}

	logger.Error(ctx, "click_consumer: message sent to DLQ (exceeded max delivery)",
		"msg_id", msgID, "dlq", streamkey.ClickDLQ)
}

// parseClickLog converts a raw Redis stream message to an entity.ClickLog.
func parseClickLog(msg redis.XMessage) (*entity.ClickLog, error) {
	v := msg.Values

	shortURLIDStr, _ := v["short_url_id"].(string)
	shortURLID, err := strconv.ParseInt(shortURLIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse short_url_id %q: %w", shortURLIDStr, err)
	}

	isBotStr, _ := v["is_bot"].(string)
	isBot, _ := strconv.ParseBool(isBotStr)

	createdAtStr, _ := v["created_at"].(string)
	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at %q: %w", createdAtStr, err)
	}

	return &entity.ClickLog{
		ID:         strVal(v, "id"),
		ShortURLID: shortURLID,
		ShortCode:  strVal(v, "short_code"),
		CreatorID:  strVal(v, "creator_id"),
		ReferralID: strVal(v, "referral_id"),
		Referrer:   strVal(v, "referrer"),
		UserAgent:  strVal(v, "user_agent"),
		IPAddress:  strVal(v, "ip_address"),
		IsBot:      isBot,
		CreatedAt:  createdAt,
	}, nil
}

// strVal safely extracts a string value from a Redis stream message map.
func strVal(values map[string]any, key string) string {
	s, _ := values[key].(string)
	return s
}
