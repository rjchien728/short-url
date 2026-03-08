package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/service"
)

const (
	// xclaimInterval is how often the XCLAIM goroutine runs to reclaim idle PEL messages.
	xclaimInterval = 10 * time.Second

	// idleThreshold is the minimum idle time before a PEL message is reclaimed.
	idleThreshold = 30 * time.Second
)

// ClickConsumer reads click log events from stream:click-log in batches and
// delegates to ClickWorkerService. Failed batches are not ACKed so they stay
// in the PEL for retry via XCLAIM. Messages that exceed maxDelivery are moved
// to stream:click-dlq (dead letter queue) and then ACKed.
type ClickConsumer struct {
	rdb          *redis.Client
	clickService service.ClickWorkerService
	groupName    string
	consumer     string
	batchSize    int64
	maxDelivery  int64
}

// NewClickConsumer creates a ClickConsumer and ensures the consumer group exists.
// Panics if the group cannot be created (any error except BUSYGROUP).
func NewClickConsumer(rdb *redis.Client, svc service.ClickWorkerService, groupName, consumerName string, batchSize, maxDelivery int) *ClickConsumer {
	err := rdb.XGroupCreateMkStream(context.Background(), clickStream, groupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		panic(fmt.Sprintf("click_consumer: failed to create consumer group: %v", err))
	}
	return &ClickConsumer{
		rdb:          rdb,
		clickService: svc,
		groupName:    groupName,
		consumer:     consumerName,
		batchSize:    int64(batchSize),
		maxDelivery:  int64(maxDelivery),
	}
}

// Run starts the main read loop and the XCLAIM goroutine concurrently.
// Both goroutines exit when ctx is cancelled.
func (c *ClickConsumer) Run(ctx context.Context) error {
	slog.Info("click_consumer: started", "group", c.groupName, "consumer", c.consumer)

	// Start XCLAIM goroutine for periodic PEL reclaim.
	go c.reclaimLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("click_consumer: stopped")
			return nil
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.groupName,
			Consumer: c.consumer,
			Streams:  []string{clickStream, ">"},
			Count:    c.batchSize,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				slog.Info("click_consumer: stopped")
				return nil
			}
			slog.Error("click_consumer: XREADGROUP error", "error", err)
			continue
		}

		for _, stream := range streams {
			if len(stream.Messages) == 0 {
				continue
			}
			c.processBatch(ctx, stream.Messages)
		}
	}
}

// processBatch parses a slice of stream messages, calls ProcessBatch, and ACKs
// only on success. On failure the messages stay in the PEL for retry.
func (c *ClickConsumer) processBatch(ctx context.Context, msgs []redis.XMessage) {
	logs := make([]*entity.ClickLog, 0, len(msgs))
	for _, msg := range msgs {
		log, err := parseClickLog(msg)
		if err != nil {
			// Malformed message: ACK to avoid infinite retry, log for observability.
			slog.Error("click_consumer: parse message failed, ACKing malformed message",
				"msg_id", msg.ID, "error", err)
			_ = c.rdb.XAck(ctx, clickStream, c.groupName, msg.ID).Err()
			continue
		}
		logs = append(logs, log)
	}

	if len(logs) == 0 {
		return
	}

	// Collect IDs for ACK after successful batch insert.
	ids := make([]string, len(logs))
	for i, msg := range msgs {
		if i < len(ids) {
			ids[i] = msg.ID
		}
	}

	if err := c.clickService.ProcessBatch(ctx, logs); err != nil {
		// Do NOT ACK — messages stay in PEL and will be retried via XCLAIM.
		slog.Error("click_consumer: ProcessBatch failed, messages left in PEL",
			"count", len(logs), "error", err)
		return
	}

	// Success — ACK all messages in the batch.
	if err := c.rdb.XAck(ctx, clickStream, c.groupName, ids...).Err(); err != nil {
		slog.Error("click_consumer: XACK failed after successful batch",
			"count", len(ids), "error", err)
		return
	}
	slog.Info("click_consumer: batch processed", "count", len(logs))
}

// reclaimLoop runs on a fixed interval to reclaim idle PEL messages and move
// messages that exceeded maxDelivery to the dead letter queue.
func (c *ClickConsumer) reclaimLoop(ctx context.Context) {
	ticker := time.NewTicker(xclaimInterval)
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
func (c *ClickConsumer) reclaim(ctx context.Context) {
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: clickStream,
		Group:  c.groupName,
		Start:  "-",
		End:    "+",
		Count:  c.batchSize,
		Idle:   idleThreshold,
	}).Result()
	if err != nil {
		slog.Error("click_consumer: XPendingExt error", "error", err)
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
			Stream:   clickStream,
			Group:    c.groupName,
			Consumer: c.consumer,
			MinIdle:  idleThreshold,
			Messages: []string{p.ID},
		}).Err(); err != nil {
			slog.Error("click_consumer: XClaim failed", "msg_id", p.ID, "error", err)
		}
	}
}

// sendToDLQ moves a message to stream:click-dlq, then ACKs it from the main stream.
func (c *ClickConsumer) sendToDLQ(ctx context.Context, msgID string) {
	// Read the original message to copy its payload to the DLQ.
	msgs, err := c.rdb.XRange(ctx, clickStream, msgID, msgID).Result()
	if err != nil || len(msgs) == 0 {
		slog.Error("click_consumer: cannot read message for DLQ transfer",
			"msg_id", msgID, "error", err)
		// ACK anyway to avoid infinite loop.
		_ = c.rdb.XAck(ctx, clickStream, c.groupName, msgID).Err()
		return
	}

	// Write to DLQ with original payload preserved.
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: clickDLQ,
		ID:     "*",
		Values: msgs[0].Values,
	}).Err(); err != nil {
		slog.Error("click_consumer: failed to write to DLQ", "msg_id", msgID, "error", err)
	}

	// ACK the original to remove it from PEL.
	if err := c.rdb.XAck(ctx, clickStream, c.groupName, msgID).Err(); err != nil {
		slog.Error("click_consumer: XACK after DLQ failed", "msg_id", msgID, "error", err)
	}

	slog.Error("click_consumer: message sent to DLQ (exceeded max delivery)",
		"msg_id", msgID, "dlq", clickDLQ)
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
