package ogworker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/mock"
	"github.com/rjchien728/short-url/internal/service/ogworker"
)

func newOGService(t *testing.T) (
	*ogworker.Service,
	*mock.MockShortURLRepository,
	*mock.MockURLCache,
	*mock.MockOGFetcher,
	*mock.MockEventPublisher,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockFetcher := mock.NewMockOGFetcher(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	svc := ogworker.New(mockRepo, mockCache, mockFetcher, mockPub)
	return svc, mockRepo, mockCache, mockFetcher, mockPub
}

func TestOGWorkerService_ProcessTask_FetchSuccess(t *testing.T) {
	svc, mockRepo, mockCache, mockFetcher, _ := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 1, ShortCode: "abc123", LongURL: "https://example.com", RetryCount: 0}
	metadata := &entity.OGMetadata{Title: "Example", Description: "A site", Image: "https://img.com/a.png"}

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(metadata, nil)
	mockRepo.EXPECT().UpdateOGMetadata(ctx, task.ShortURLID, metadata).Return(nil)
	// cache must be invalidated after successful DB write
	mockCache.EXPECT().Delete(ctx, task.ShortCode).Return(nil)

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchSuccess_CacheDeleteFails(t *testing.T) {
	svc, mockRepo, mockCache, mockFetcher, _ := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 1, ShortCode: "abc123", LongURL: "https://example.com", RetryCount: 0}
	metadata := &entity.OGMetadata{Title: "Example"}

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(metadata, nil)
	mockRepo.EXPECT().UpdateOGMetadata(ctx, task.ShortURLID, metadata).Return(nil)
	// cache delete failure is non-fatal — ProcessTask must still succeed
	mockCache.EXPECT().Delete(ctx, task.ShortCode).Return(errors.New("redis down"))

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_ReEnqueue(t *testing.T) {
	svc, _, _, mockFetcher, mockPub := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 2, ShortCode: "xyz789", LongURL: "https://example.com", RetryCount: 1}
	fetchErr := errors.New("connection timeout")

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, fetchErr)
	// expect re-enqueue with RetryCount incremented to 2 and ShortCode forwarded
	mockPub.EXPECT().PublishOGFetchTask(ctx, &entity.OGFetchTask{
		ShortURLID: task.ShortURLID,
		ShortCode:  task.ShortCode,
		LongURL:    task.LongURL,
		RetryCount: 2,
	}).Return(nil)
	// DB not written → cache must NOT be invalidated

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_MaxRetry_MarkFailed(t *testing.T) {
	svc, mockRepo, mockCache, mockFetcher, _ := newOGService(t)
	ctx := context.Background()

	// RetryCount == maxRetry (3) — no more re-enqueues
	task := &entity.OGFetchTask{ShortURLID: 3, ShortCode: "def456", LongURL: "https://example.com", RetryCount: 3}
	fetchErr := errors.New("connection timeout")

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, fetchErr)
	mockRepo.EXPECT().UpdateOGMetadata(ctx, task.ShortURLID, &entity.OGMetadata{FetchFailed: true}).Return(nil)
	// cache must be invalidated after marking fetch-failed
	mockCache.EXPECT().Delete(ctx, task.ShortCode).Return(nil)

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_ReEnqueueError(t *testing.T) {
	svc, _, _, mockFetcher, mockPub := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 4, ShortCode: "ghi012", LongURL: "https://example.com", RetryCount: 0}

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, errors.New("timeout"))
	mockPub.EXPECT().PublishOGFetchTask(ctx, gomock.Any()).Return(errors.New("redis down"))

	err := svc.ProcessTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-enqueue")
}
