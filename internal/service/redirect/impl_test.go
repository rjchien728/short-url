package redirect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/mock"
	"github.com/rjchien728/short-url/internal/service/redirect"
)

func newService(t *testing.T) (
	*redirect.Service,
	*mock.MockShortURLRepository,
	*mock.MockURLCache,
	*mock.MockEventPublisher,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	svc := redirect.New(mockRepo, mockCache, mockPub)
	return svc, mockRepo, mockCache, mockPub
}

func TestRedirectService_Resolve_CacheHit(t *testing.T) {
	svc, mockRepo, mockCache, _ := newService(t)
	ctx := context.Background()

	cached := &entity.ShortURL{
		ShortCode: "abc1234567",
		LongURL:   "https://example.com",
	}

	mockCache.EXPECT().Get(ctx, "abc1234567").Return(cached, nil)
	// repo must NOT be called on cache hit
	mockRepo.EXPECT().FindByShortCode(gomock.Any(), gomock.Any()).Times(0)

	result, err := svc.Resolve(ctx, "abc1234567")

	require.NoError(t, err)
	assert.Equal(t, cached, result)
}

func TestRedirectService_Resolve_CacheMiss_DBHit(t *testing.T) {
	svc, mockRepo, mockCache, _ := newService(t)
	ctx := context.Background()

	fromDB := &entity.ShortURL{
		ShortCode: "abc1234567",
		LongURL:   "https://example.com",
	}

	mockCache.EXPECT().Get(ctx, "abc1234567").Return(nil, nil) // cache miss
	mockRepo.EXPECT().FindByShortCode(ctx, "abc1234567").Return(fromDB, nil)
	mockCache.EXPECT().Set(ctx, "abc1234567", fromDB).Return(nil) // backfill

	result, err := svc.Resolve(ctx, "abc1234567")

	require.NoError(t, err)
	assert.Equal(t, fromDB, result)
}

func TestRedirectService_Resolve_NotFound(t *testing.T) {
	svc, mockRepo, mockCache, _ := newService(t)
	ctx := context.Background()

	mockCache.EXPECT().Get(ctx, "notexist1").Return(nil, nil)
	mockRepo.EXPECT().FindByShortCode(ctx, "notexist1").Return(nil, entity.ErrNotFound)

	_, err := svc.Resolve(ctx, "notexist1")

	require.Error(t, err)
	assert.True(t, errors.Is(err, entity.ErrNotFound))
}

func TestRedirectService_Resolve_Expired(t *testing.T) {
	svc, _, mockCache, _ := newService(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	expired := &entity.ShortURL{
		ShortCode: "expiredcod",
		LongURL:   "https://example.com",
		ExpiresAt: &past,
	}

	// expired URL may come from cache or DB — here it comes from cache
	mockCache.EXPECT().Get(ctx, "expiredcod").Return(expired, nil)

	_, err := svc.Resolve(ctx, "expiredcod")

	require.Error(t, err)
	assert.True(t, errors.Is(err, entity.ErrExpired))
}

func TestRedirectService_RecordClick_HappyPath(t *testing.T) {
	svc, _, _, mockPub := newService(t)
	ctx := context.Background()

	clickLog := &entity.ClickLog{
		ID:        "uuid-1",
		ShortCode: "abc1234567",
	}

	mockPub.EXPECT().PublishClickEvent(ctx, clickLog).Return(nil)

	err := svc.RecordClick(ctx, clickLog)
	require.NoError(t, err)
}

func TestRedirectService_RecordClick_PublisherFailure_DoesNotError(t *testing.T) {
	svc, _, _, mockPub := newService(t)
	ctx := context.Background()

	clickLog := &entity.ClickLog{
		ID:        "uuid-2",
		ShortCode: "abc1234567",
	}

	// publisher fails — RecordClick must still return nil
	mockPub.EXPECT().PublishClickEvent(ctx, clickLog).Return(errors.New("redis timeout"))

	err := svc.RecordClick(ctx, clickLog)
	require.NoError(t, err)
}
