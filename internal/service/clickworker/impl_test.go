package clickworker_test

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
	"github.com/rjchien728/short-url/internal/service/clickworker"
)

func TestClickWorkerService_ProcessBatch_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockClickLogRepository(ctrl)
	svc := clickworker.New(mockRepo)
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{ID: "uuid-1", ShortCode: "abc1234567", CreatedAt: time.Now()},
		{ID: "uuid-2", ShortCode: "abc1234567", CreatedAt: time.Now()},
	}

	mockRepo.EXPECT().BatchCreate(ctx, logs).Return(nil)

	err := svc.ProcessBatch(ctx, logs)
	require.NoError(t, err)
}

func TestClickWorkerService_ProcessBatch_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockClickLogRepository(ctrl)
	svc := clickworker.New(mockRepo)
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{ID: "uuid-3", ShortCode: "abc1234567", CreatedAt: time.Now()},
	}

	mockRepo.EXPECT().BatchCreate(ctx, logs).Return(errors.New("db connection refused"))

	err := svc.ProcessBatch(ctx, logs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clickworker.ProcessBatch")
}
