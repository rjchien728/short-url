package consumer_test

// Integration tests for ClickConsumer.
//
// Prerequisites: a running Redis instance.
// Set REDIS_STREAM_URL environment variable (or use .env file) before running.
//
// Run with:
//   make test-integration
// or:
//   REDIS_STREAM_URL=redis://... go test ./internal/consumer/...

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/rjchien728/short-url/internal/consumer"
	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/mock"
)

const clickTestStream = "stream:click-log"
const clickTestDLQ = "stream:click-dlq"
const clickTestConsumer = "test-click-consumer"

type ClickConsumerSuite struct {
	suite.Suite
	rdb *redis.Client
}

func (s *ClickConsumerSuite) SetupSuite() {
	streamURL := os.Getenv("REDIS_STREAM_URL")
	if streamURL == "" {
		s.T().Skip("REDIS_STREAM_URL not set — skipping consumer integration tests")
	}

	opts, err := redis.ParseURL(streamURL)
	s.Require().NoError(err)
	s.rdb = redis.NewClient(opts)

	ctx := context.Background()
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		s.T().Skipf("cannot connect to Redis: %v", err)
	}
}

func (s *ClickConsumerSuite) TearDownSuite() {
	if s.rdb != nil {
		_ = s.rdb.Close()
	}
}

func (s *ClickConsumerSuite) SetupTest() {
	ctx := context.Background()
	s.rdb.Del(ctx, clickTestStream, clickTestDLQ)
}

// newClickConsumer creates a ClickConsumer with a unique group name per test.
// Uses a short block timeout so consumer.Run exits quickly after ctx is cancelled.
func (s *ClickConsumerSuite) newClickConsumer(svc interface {
	ProcessBatch(context.Context, []*entity.ClickLog) error
}, groupName string) *consumer.ClickConsumer {
	return consumer.NewClickConsumer(s.rdb, svc, groupName, clickTestConsumer, 10, 3).
		WithBlockTimeout(200 * time.Millisecond)
}

// publishClickEvent writes a well-formed click event to the stream.
func (s *ClickConsumerSuite) publishClickEvent(ctx context.Context, id string) string {
	msgID, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: clickTestStream,
		ID:     "*",
		Values: map[string]any{
			"id":           id,
			"short_url_id": "1001",
			"short_code":   "abc1234567",
			"creator_id":   "user_01",
			"referral_id":  "",
			"referrer":     "https://google.com",
			"user_agent":   "Mozilla/5.0",
			"ip_address":   "1.2.3.4",
			"is_bot":       "false",
			"created_at":   time.Now().UTC().Format(time.RFC3339),
		},
	}).Result()
	s.Require().NoError(err)
	return msgID
}

// --- Test cases ---

// TestProcessBatch_Success verifies that messages are ACKed after a successful ProcessBatch.
func (s *ClickConsumerSuite) TestProcessBatch_Success() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockClickWorkerService(ctrl)
	svc.EXPECT().
		ProcessBatch(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	groupName := "test-click-success-group"
	c := s.newClickConsumer(svc, groupName)

	msgID := s.publishClickEvent(ctx, "click-id-001")

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- c.Run(consumerCtx)
	}()

	time.Sleep(200 * time.Millisecond)
	consumerCancel()
	<-doneCh

	// Verify message was ACKed (PEL should be empty).
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: clickTestStream,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	s.Require().NoError(err)
	s.Empty(pending, "message %s should have been ACKed on success", msgID)
}

// TestProcessBatch_Failure verifies that messages are NOT ACKed when ProcessBatch fails,
// leaving them in the PEL for retry.
func (s *ClickConsumerSuite) TestProcessBatch_Failure() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockClickWorkerService(ctrl)
	svc.EXPECT().
		ProcessBatch(gomock.Any(), gomock.Any()).
		Return(errors.New("db error")).
		Times(1)

	groupName := "test-click-failure-group"
	c := s.newClickConsumer(svc, groupName)

	msgID := s.publishClickEvent(ctx, "click-id-002")

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- c.Run(consumerCtx)
	}()

	time.Sleep(200 * time.Millisecond)
	consumerCancel()
	<-doneCh

	// Message should still be in PEL (not ACKed).
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: clickTestStream,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	s.Require().NoError(err)
	s.Require().Len(pending, 1, "message %s should remain in PEL after failure", msgID)
	s.Equal(msgID, pending[0].ID)
}

// TestProcessBatch_ParsedCorrectly verifies that stream fields are correctly parsed into ClickLog.
func (s *ClickConsumerSuite) TestProcessBatch_ParsedCorrectly() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockClickWorkerService(ctrl)

	var capturedLogs []*entity.ClickLog
	svc.EXPECT().
		ProcessBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, logs []*entity.ClickLog) error {
			capturedLogs = logs
			return nil
		}).
		Times(1)

	groupName := "test-click-parse-group"
	c := s.newClickConsumer(svc, groupName)

	now := time.Now().UTC().Truncate(time.Second)
	_, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: clickTestStream,
		ID:     "*",
		Values: map[string]any{
			"id":           "test-uuid-parse",
			"short_url_id": "8888",
			"short_code":   "parsecode1",
			"creator_id":   "creator_x",
			"referral_id":  "ref_123",
			"referrer":     "https://parse.test",
			"user_agent":   "TestAgent/1.0",
			"ip_address":   "10.0.0.1",
			"is_bot":       "true",
			"created_at":   now.Format(time.RFC3339),
		},
	}).Result()
	s.Require().NoError(err)

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- c.Run(consumerCtx)
	}()

	time.Sleep(200 * time.Millisecond)
	consumerCancel()
	<-doneCh

	s.Require().Len(capturedLogs, 1)
	log := capturedLogs[0]
	s.Equal("test-uuid-parse", log.ID)
	s.Equal(int64(8888), log.ShortURLID)
	s.Equal("parsecode1", log.ShortCode)
	s.Equal("creator_x", log.CreatorID)
	s.Equal("ref_123", log.ReferralID)
	s.Equal("https://parse.test", log.Referrer)
	s.Equal("TestAgent/1.0", log.UserAgent)
	s.Equal("10.0.0.1", log.IPAddress)
	s.True(log.IsBot)
	s.Equal(now, log.CreatedAt)
}

// TestDLQ_ExceedMaxDelivery verifies that a message exceeding maxDelivery is moved to the DLQ.
//
// Setup: manually bump the delivery count to 4 (> maxDelivery=3) via XClaim with MinIdle=0,
// then start the consumer with short reclaimInterval/idleThreshold so the reclaim loop fires
// quickly without waiting real-world 10s/30s.
func (s *ClickConsumerSuite) TestDLQ_ExceedMaxDelivery() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	// Service always fails so the message is never ACKed and stays in PEL.
	svc := mock.NewMockClickWorkerService(ctrl)
	svc.EXPECT().
		ProcessBatch(gomock.Any(), gomock.Any()).
		Return(errors.New("persistent db error")).
		AnyTimes()

	groupName := "test-click-dlq-group"

	// Ensure group exists before publishing.
	_ = s.rdb.XGroupCreateMkStream(ctx, clickTestStream, groupName, "0").Err()

	// Publish a message.
	msgID := s.publishClickEvent(ctx, "click-dlq-test")

	// Bump delivery count to 4 (> maxDelivery=3) using XClaim with MinIdle=0.
	// First read puts it in PEL with delivery_count=1, each XClaim increments by 1.
	_, _ = s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: "dummy-consumer-0",
		Streams:  []string{clickTestStream, ">"},
		Count:    1,
		Block:    0,
	}).Result()

	for i := 0; i < 3; i++ {
		_, _ = s.rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream:   clickTestStream,
			Group:    groupName,
			Consumer: fmt.Sprintf("dummy-consumer-%d", i+1),
			MinIdle:  0,
			Messages: []string{msgID},
		}).Result()
	}

	// Confirm delivery count is now > maxDelivery before starting consumer.
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: clickTestStream,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	s.Require().NoError(err)
	s.Require().Len(pending, 1)
	s.Greater(pending[0].RetryCount, int64(3), "delivery count should exceed maxDelivery=3")

	// Use short intervals so the reclaim loop fires quickly in the test environment.
	c := consumer.NewClickConsumer(s.rdb, svc, groupName, clickTestConsumer, 10, 3).
		WithReclaimInterval(200 * time.Millisecond).
		WithIdleThreshold(100 * time.Millisecond).
		WithBlockTimeout(200 * time.Millisecond)

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() { doneCh <- c.Run(consumerCtx) }()

	// Poll until the DLQ receives the message (should happen within ~500ms).
	var dlqMsgs []redis.XMessage
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dlqMsgs, _ = s.rdb.XRange(ctx, clickTestDLQ, "-", "+").Result()
		if len(dlqMsgs) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	consumerCancel()
	<-doneCh

	// Verify message is now in DLQ.
	s.NotEmpty(dlqMsgs, "message should have been moved to DLQ")

	// Verify original is ACKed (no longer in PEL).
	pendingAfter, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: clickTestStream,
		Group:  groupName,
		Start:  msgID,
		End:    msgID,
		Count:  1,
	}).Result()
	s.Require().NoError(err)
	s.Empty(pendingAfter, "original message should be ACKed after DLQ transfer")
}

func TestClickConsumer(t *testing.T) {
	suite.Run(t, new(ClickConsumerSuite))
}
