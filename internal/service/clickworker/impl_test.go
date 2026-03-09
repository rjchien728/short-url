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
	mockGeoIP := mock.NewMockGeoIPResolver(ctrl)
	svc := clickworker.New(mockRepo, mockGeoIP)
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{ID: "uuid-1", ShortCode: "abc1234567", IPAddress: "1.2.3.4", CreatedAt: time.Now()},
		{ID: "uuid-2", ShortCode: "abc1234567", IPAddress: "5.6.7.8", CreatedAt: time.Now()},
	}

	mockGeoIP.EXPECT().LookupCountry("1.2.3.4").Return("TW", nil)
	mockGeoIP.EXPECT().LookupCountry("5.6.7.8").Return("US", nil)
	mockRepo.EXPECT().BatchCreate(ctx, logs).Return(nil)

	err := svc.ProcessBatch(ctx, logs)
	require.NoError(t, err)

	tw := "TW"
	us := "US"
	assert.Equal(t, &tw, logs[0].CountryCode)
	assert.Equal(t, &us, logs[1].CountryCode)
}

func TestClickWorkerService_ProcessBatch_GeoIPFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockClickLogRepository(ctrl)
	mockGeoIP := mock.NewMockGeoIPResolver(ctrl)
	svc := clickworker.New(mockRepo, mockGeoIP)
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{ID: "uuid-3", ShortCode: "abc1234567", IPAddress: "1.2.3.4", CreatedAt: time.Now()},
	}

	// GeoIP lookup fails: should log warn and continue, CountryCode stays nil.
	mockGeoIP.EXPECT().LookupCountry("1.2.3.4").Return("", errors.New("mmdb read error"))
	mockRepo.EXPECT().BatchCreate(ctx, logs).Return(nil)

	err := svc.ProcessBatch(ctx, logs)
	require.NoError(t, err)
	assert.Nil(t, logs[0].CountryCode)
}

func TestClickWorkerService_ProcessBatch_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mock.NewMockClickLogRepository(ctrl)
	mockGeoIP := mock.NewMockGeoIPResolver(ctrl)
	svc := clickworker.New(mockRepo, mockGeoIP)
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{ID: "uuid-4", ShortCode: "abc1234567", IPAddress: "1.2.3.4", CreatedAt: time.Now()},
	}

	mockGeoIP.EXPECT().LookupCountry("1.2.3.4").Return("TW", nil)
	mockRepo.EXPECT().BatchCreate(ctx, logs).Return(errors.New("db connection refused"))

	err := svc.ProcessBatch(ctx, logs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clickworker.ProcessBatch")
}
