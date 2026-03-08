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
	*mock.MockOGFetcher,
	*mock.MockEventPublisher,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockFetcher := mock.NewMockOGFetcher(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	svc := ogworker.New(mockRepo, mockFetcher, mockPub)
	return svc, mockRepo, mockFetcher, mockPub
}

func TestOGWorkerService_ProcessTask_FetchSuccess(t *testing.T) {
	svc, mockRepo, mockFetcher, _ := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 1, LongURL: "https://example.com", RetryCount: 0}
	metadata := &entity.OGMetadata{Title: "Example", Description: "A site", Image: "https://img.com/a.png"}

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(metadata, nil)
	mockRepo.EXPECT().UpdateOGMetadata(ctx, task.ShortURLID, metadata).Return(nil)

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_ReEnqueue(t *testing.T) {
	svc, _, mockFetcher, mockPub := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 2, LongURL: "https://example.com", RetryCount: 1}
	fetchErr := errors.New("connection timeout")

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, fetchErr)
	// expect re-enqueue with RetryCount incremented to 2
	mockPub.EXPECT().PublishOGFetchTask(ctx, &entity.OGFetchTask{
		ShortURLID: task.ShortURLID,
		LongURL:    task.LongURL,
		RetryCount: 2,
	}).Return(nil)

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_MaxRetry_MarkFailed(t *testing.T) {
	svc, mockRepo, mockFetcher, _ := newOGService(t)
	ctx := context.Background()

	// RetryCount == maxRetry (3) — no more re-enqueues
	task := &entity.OGFetchTask{ShortURLID: 3, LongURL: "https://example.com", RetryCount: 3}
	fetchErr := errors.New("connection timeout")

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, fetchErr)
	mockRepo.EXPECT().UpdateOGMetadata(ctx, task.ShortURLID, &entity.OGMetadata{FetchFailed: true}).Return(nil)

	err := svc.ProcessTask(ctx, task)
	require.NoError(t, err)
}

func TestOGWorkerService_ProcessTask_FetchFail_ReEnqueueError(t *testing.T) {
	svc, _, mockFetcher, mockPub := newOGService(t)
	ctx := context.Background()

	task := &entity.OGFetchTask{ShortURLID: 4, LongURL: "https://example.com", RetryCount: 0}

	mockFetcher.EXPECT().Fetch(ctx, task.LongURL).Return(nil, errors.New("timeout"))
	mockPub.EXPECT().PublishOGFetchTask(ctx, gomock.Any()).Return(errors.New("redis down"))

	err := svc.ProcessTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-enqueue")
}
