package urlsvc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domainservice "github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/mock"
	urlsvc "github.com/rjchien728/short-url/internal/service/url"
)

// uniqueViolationErr returns a pgconn.PgError with code 23505 (unique constraint violation).
func uniqueViolationErr() error {
	return &pgconn.PgError{Code: "23505", Message: "duplicate key value"}
}

func TestURLService_Create_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	mockIDGen := mock.NewMockIDGenerator(ctrl)

	svc := urlsvc.New(mockRepo, mockCache, mockPub, mockIDGen)

	ctx := context.Background()
	req := domainservice.CreateURLRequest{
		LongURL:   "https://example.com",
		CreatorID: "user_01",
	}

	mockIDGen.EXPECT().Generate().Return(int64(12345), nil)
	mockIDGen.EXPECT().ShortCode(int64(12345)).Return("1111111abc")
	mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
	mockPub.EXPECT().PublishOGFetchTask(ctx, gomock.Any()).Return(nil)

	result, err := svc.Create(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(12345), result.ID)
	assert.Equal(t, "https://example.com", result.LongURL)
	assert.Equal(t, "user_01", result.CreatorID)
	assert.NotEmpty(t, result.ShortCode)
}

func TestURLService_Create_UniqueViolation_RetrySuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	mockIDGen := mock.NewMockIDGenerator(ctrl)

	svc := urlsvc.New(mockRepo, mockCache, mockPub, mockIDGen)
	ctx := context.Background()
	req := domainservice.CreateURLRequest{LongURL: "https://example.com", CreatorID: "user_01"}

	// first Generate → collision, second Generate → success
	gomock.InOrder(
		mockIDGen.EXPECT().Generate().Return(int64(111), nil),
		mockIDGen.EXPECT().Generate().Return(int64(222), nil),
	)
	mockIDGen.EXPECT().ShortCode(int64(111)).Return("1111111aaa")
	mockIDGen.EXPECT().ShortCode(int64(222)).Return("1111111bbb")
	gomock.InOrder(
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(uniqueViolationErr()),
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil),
	)
	mockPub.EXPECT().PublishOGFetchTask(ctx, gomock.Any()).Return(nil)

	result, err := svc.Create(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, int64(222), result.ID)
}

func TestURLService_Create_MaxRetryExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	mockIDGen := mock.NewMockIDGenerator(ctrl)

	svc := urlsvc.New(mockRepo, mockCache, mockPub, mockIDGen)
	ctx := context.Background()
	req := domainservice.CreateURLRequest{LongURL: "https://example.com", CreatorID: "user_01"}

	// all 3 attempts fail with unique violation
	mockIDGen.EXPECT().Generate().Return(int64(1), nil).Times(3)
	mockIDGen.EXPECT().ShortCode(int64(1)).Return("1111111ccc").Times(3)
	mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(uniqueViolationErr()).Times(3)

	_, err := svc.Create(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
}

func TestURLService_Create_IDGeneratorError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	mockIDGen := mock.NewMockIDGenerator(ctrl)

	svc := urlsvc.New(mockRepo, mockCache, mockPub, mockIDGen)
	ctx := context.Background()
	req := domainservice.CreateURLRequest{LongURL: "https://example.com", CreatorID: "user_01"}

	mockIDGen.EXPECT().Generate().Return(int64(0), errors.New("clock moved backwards"))
	// ShortCode should NOT be called when Generate fails

	_, err := svc.Create(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate id")
}

func TestURLService_Create_PublisherError_StillReturnsURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockShortURLRepository(ctrl)
	mockCache := mock.NewMockURLCache(ctrl)
	mockPub := mock.NewMockEventPublisher(ctrl)
	mockIDGen := mock.NewMockIDGenerator(ctrl)

	svc := urlsvc.New(mockRepo, mockCache, mockPub, mockIDGen)
	ctx := context.Background()
	req := domainservice.CreateURLRequest{LongURL: "https://example.com", CreatorID: "user_01"}

	mockIDGen.EXPECT().Generate().Return(int64(99), nil)
	mockIDGen.EXPECT().ShortCode(int64(99)).Return("1111111ddd")
	mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
	// publisher fails — should NOT block create
	mockPub.EXPECT().PublishOGFetchTask(ctx, gomock.Any()).Return(errors.New("redis down"))

	result, err := svc.Create(ctx, req)

	// create must succeed even if publisher fails
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(99), result.ID)
}
