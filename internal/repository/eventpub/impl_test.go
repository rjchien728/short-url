package eventpub_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/repository/eventpub"
)

type EventPubSuite struct {
	suite.Suite
	mr        *miniredis.Miniredis
	publisher *eventpub.Publisher
	rdb       *redis.Client
}

func (s *EventPubSuite) SetupSuite() {
	mr, err := miniredis.Run()
	s.Require().NoError(err)
	s.mr = mr

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s.rdb = rdb
	s.publisher = eventpub.NewPublisher(rdb)
}

func (s *EventPubSuite) TearDownSuite() {
	_ = s.rdb.Close()
	s.mr.Close()
}

func (s *EventPubSuite) SetupTest() {
	s.mr.FlushAll()
}

// --- Test cases ---

func (s *EventPubSuite) TestPublishOGFetchTask_FieldsCorrect() {
	ctx := context.Background()

	task := &entity.OGFetchTask{
		ShortURLID: 1001,
		LongURL:    "https://example.com/page",
	}

	err := s.publisher.PublishOGFetchTask(ctx, task)
	s.Require().NoError(err)

	// Read the message back from the stream.
	msgs, err := s.rdb.XRange(ctx, "stream:og-fetch", "-", "+").Result()
	s.Require().NoError(err)
	s.Require().Len(msgs, 1)

	fields := msgs[0].Values
	s.Equal("1001", fields["short_url_id"])
	s.Equal("https://example.com/page", fields["long_url"])
	s.Equal("0", fields["retry_count"])
}

func (s *EventPubSuite) TestPublishOGFetchTask_RetryCount() {
	ctx := context.Background()

	task := &entity.OGFetchTask{
		ShortURLID: 1002,
		LongURL:    "https://example.com/retry",
		RetryCount: 2,
	}

	err := s.publisher.PublishOGFetchTask(ctx, task)
	s.Require().NoError(err)

	msgs, err := s.rdb.XRange(ctx, "stream:og-fetch", "-", "+").Result()
	s.Require().NoError(err)
	s.Require().Len(msgs, 1)
	s.Equal("2", msgs[0].Values["retry_count"])
}

func (s *EventPubSuite) TestPublishClickEvent_FieldsCorrect() {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	event := &entity.ClickLog{
		ID:         "550e8400-e29b-41d4-a716-446655440001",
		ShortURLID: 2001,
		ShortCode:  "abc1234567",
		CreatorID:  "user_01",
		ReferralID: "ref_abc",
		Referrer:   "https://google.com",
		UserAgent:  "Mozilla/5.0",
		IPAddress:  "1.2.3.4",
		IsBot:      false,
		CreatedAt:  now,
	}

	err := s.publisher.PublishClickEvent(ctx, event)
	s.Require().NoError(err)

	msgs, err := s.rdb.XRange(ctx, "stream:click-log", "-", "+").Result()
	s.Require().NoError(err)
	s.Require().Len(msgs, 1)

	fields := msgs[0].Values
	s.Equal(event.ID, fields["id"])
	s.Equal("2001", fields["short_url_id"])
	s.Equal("abc1234567", fields["short_code"])
	s.Equal("user_01", fields["creator_id"])
	s.Equal("ref_abc", fields["referral_id"])
	s.Equal("https://google.com", fields["referrer"])
	s.Equal("Mozilla/5.0", fields["user_agent"])
	s.Equal("1.2.3.4", fields["ip_address"])
	s.Equal("false", fields["is_bot"])
	s.Equal("2026-01-01T12:00:00Z", fields["created_at"])
}

func (s *EventPubSuite) TestPublishClickEvent_IsBot_True() {
	ctx := context.Background()

	event := &entity.ClickLog{
		ID:         "550e8400-e29b-41d4-a716-446655440002",
		ShortURLID: 3001,
		ShortCode:  "bot0000001",
		CreatorID:  "user_02",
		IsBot:      true,
		CreatedAt:  time.Now().UTC(),
	}

	err := s.publisher.PublishClickEvent(ctx, event)
	s.Require().NoError(err)

	msgs, err := s.rdb.XRange(ctx, "stream:click-log", "-", "+").Result()
	s.Require().NoError(err)
	s.Require().Len(msgs, 1)
	s.Equal("true", msgs[0].Values["is_bot"])
}

func TestEventPub(t *testing.T) {
	suite.Run(t, new(EventPubSuite))
}
