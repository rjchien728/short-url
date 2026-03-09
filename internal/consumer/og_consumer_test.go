package consumer_test

// Integration tests for OGConsumer.
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

const ogTestStream = "stream:og-fetch"
const ogTestConsumer = "test-og-consumer"

type OGConsumerSuite struct {
	suite.Suite
	rdb *redis.Client
}

func (s *OGConsumerSuite) SetupSuite() {
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

func (s *OGConsumerSuite) TearDownSuite() {
	if s.rdb != nil {
		_ = s.rdb.Close()
	}
}

func (s *OGConsumerSuite) SetupTest() {
	// Clean up stream and consumer group before each test.
	ctx := context.Background()
	s.rdb.Del(ctx, ogTestStream)
}

// newOGConsumer creates a fresh consumer with a unique group name per test to avoid PEL collisions.
// Uses a short block timeout so consumer.Run exits quickly after ctx is cancelled.
func (s *OGConsumerSuite) newOGConsumer(svc interface {
	ProcessTask(context.Context, *entity.OGFetchTask) error
}, groupName string) *consumer.OGConsumer {
	return consumer.NewOGConsumer(s.rdb, svc, groupName, ogTestConsumer).
		WithBlockTimeout(200 * time.Millisecond)
}

// publishOGTask writes a raw og-fetch message to the stream (bypassing eventpub to keep test self-contained).
func (s *OGConsumerSuite) publishOGTask(ctx context.Context, shortURLID int64, longURL string, retryCount int) string {
	res, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: ogTestStream,
		ID:     "*",
		Values: map[string]any{
			"short_url_id": "1001",
			"long_url":     longURL,
			"retry_count":  "0",
		},
	}).Result()
	s.Require().NoError(err)
	_ = shortURLID
	_ = retryCount
	return res
}

// --- Test cases ---

// TestProcessTask_Success verifies that a message is ACKed after successful ProcessTask.
func (s *OGConsumerSuite) TestProcessTask_Success() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockOGWorkerService(ctrl)

	// Expect exactly one ProcessTask call, returning nil (success).
	svc.EXPECT().
		ProcessTask(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	groupName := "test-og-success-group"
	c := s.newOGConsumer(svc, groupName)

	msgID := s.publishOGTask(ctx, 1001, "https://example.com", 0)

	// Run the consumer in background; cancel after it processes the message.
	doneCh := make(chan error, 1)
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	go func() {
		doneCh <- c.Run(consumerCtx)
	}()

	// Wait briefly for the consumer to pick up the message, then stop it.
	time.Sleep(200 * time.Millisecond)
	consumerCancel()
	<-doneCh

	// Verify the message is no longer in PEL (was ACKed).
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: ogTestStream,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	s.Require().NoError(err)
	s.Empty(pending, "message %s should have been ACKed", msgID)
}

// TestProcessTask_ServiceError verifies that even when ProcessTask returns an error,
// the message is still ACKed (OG fetch failures are non-fatal).
func (s *OGConsumerSuite) TestProcessTask_ServiceError() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockOGWorkerService(ctrl)

	// ProcessTask returns an error — consumer should still ACK.
	svc.EXPECT().
		ProcessTask(gomock.Any(), gomock.Any()).
		Return(errors.New("fetch failed")).
		Times(1)

	groupName := "test-og-error-group"
	c := s.newOGConsumer(svc, groupName)

	msgID := s.publishOGTask(ctx, 1002, "https://bad-url.example.com", 0)

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- c.Run(consumerCtx)
	}()

	time.Sleep(200 * time.Millisecond)
	consumerCancel()
	<-doneCh

	// Verify message was ACKed despite the service error.
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: ogTestStream,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	s.Require().NoError(err)
	s.Empty(pending, "message %s should have been ACKed even on service error", msgID)
}

// TestProcessTask_ParsedCorrectly verifies that stream fields are correctly parsed into OGFetchTask.
func (s *OGConsumerSuite) TestProcessTask_ParsedCorrectly() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	svc := mock.NewMockOGWorkerService(ctrl)

	var capturedTask *entity.OGFetchTask
	svc.EXPECT().
		ProcessTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, task *entity.OGFetchTask) error {
			capturedTask = task
			return nil
		}).
		Times(1)

	groupName := "test-og-parse-group"
	c := s.newOGConsumer(svc, groupName)

	// Publish with explicit field values.
	_, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: ogTestStream,
		ID:     "*",
		Values: map[string]any{
			"short_url_id": "9999",
			"long_url":     "https://parse-test.com",
			"retry_count":  "2",
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

	s.Require().NotNil(capturedTask)
	s.Equal(int64(9999), capturedTask.ShortURLID)
	s.Equal("https://parse-test.com", capturedTask.LongURL)
	s.Equal(2, capturedTask.RetryCount)
}

func TestOGConsumer(t *testing.T) {
	suite.Run(t, new(OGConsumerSuite))
}
