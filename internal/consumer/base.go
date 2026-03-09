package consumer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rjchien728/short-url/internal/pkg/logger"
)

// MessageProcessor handles a batch of stream messages.
// ACK responsibility belongs entirely to the implementation — RunLoop never ACKs.
type MessageProcessor interface {
	ProcessMessages(ctx context.Context, rdb *redis.Client, msgs []redis.XMessage) error
}

// RunLoopOptions configures the shared XReadGroup event loop.
type RunLoopOptions struct {
	// Stream is the Redis stream name to read from.
	Stream string
	// Group is the consumer group name.
	Group string
	// Consumer is the consumer instance name.
	Consumer string
	// Count is the max number of messages to fetch per XReadGroup call.
	Count int64
	// BlockTimeout is how long XReadGroup blocks waiting for new messages.
	BlockTimeout time.Duration
	// Processor handles each batch of messages (including ACK decisions).
	Processor MessageProcessor
}

// RunLoop runs the XReadGroup event loop until ctx is cancelled.
// It creates the consumer group (BUSYGROUP ignored) then enters the read loop.
// All ACK logic is delegated to opts.Processor.
func RunLoop(ctx context.Context, rdb *redis.Client, opts RunLoopOptions) error {
	// Ensure consumer group exists; BUSYGROUP means it already exists — safe to ignore.
	err := rdb.XGroupCreateMkStream(ctx, opts.Stream, opts.Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("consumer: failed to create group %q on stream %q: %w", opts.Group, opts.Stream, err)
	}

	logger.Info(ctx, "consumer: run loop started",
		"stream", opts.Stream, "group", opts.Group, "consumer", opts.Consumer)

	for {
		// Check for cancellation before blocking on Redis.
		select {
		case <-ctx.Done():
			logger.Info(ctx, "consumer: run loop stopped",
				"stream", opts.Stream, "group", opts.Group)
			return nil
		default:
		}

		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    opts.Group,
			Consumer: opts.Consumer,
			Streams:  []string{opts.Stream, ">"},
			Count:    opts.Count,
			Block:    opts.BlockTimeout,
		}).Result()
		if err != nil {
			// redis.Nil means the blocking read timed out — normal, just retry.
			if err == redis.Nil {
				continue
			}
			// Context was cancelled while blocking — clean exit.
			if ctx.Err() != nil {
				logger.Info(ctx, "consumer: run loop stopped",
					"stream", opts.Stream, "group", opts.Group)
				return nil
			}
			logger.Error(ctx, "consumer: XREADGROUP error",
				"stream", opts.Stream, "group", opts.Group, "error", err)
			continue
		}

		for _, stream := range streams {
			if len(stream.Messages) == 0 {
				continue
			}
			if err := opts.Processor.ProcessMessages(ctx, rdb, stream.Messages); err != nil {
				logger.Error(ctx, "consumer: ProcessMessages error",
					"stream", opts.Stream, "error", err)
			}
		}
	}
}
