package clicklog_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/repository/clicklog"
)

type ClickLogRepoSuite struct {
	suite.Suite
	container *tcpostgres.PostgresContainer
	repo      *clicklog.Repository
	pool      *pgxpool.Pool
}

func (s *ClickLogRepoSuite) SetupSuite() {
	ctx := context.Background()

	// click_log has no FK constraint on short_url_id, so only this migration is needed.
	const createSQL = `
		CREATE TABLE IF NOT EXISTS click_log (
			id UUID PRIMARY KEY,
			short_url_id BIGINT NOT NULL,
			short_code VARCHAR(10) NOT NULL,
			creator_id VARCHAR(50) NOT NULL,
			referral_id VARCHAR(50),
			referrer TEXT,
			user_agent TEXT,
			ip_address VARCHAR(45),
			is_bot BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.WithSQLDriver("pgx"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	s.Require().NoError(err)
	s.container = container

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)

	pool, err := pgxpool.New(ctx, dsn)
	s.Require().NoError(err)
	s.pool = pool

	_, err = pool.Exec(ctx, createSQL)
	s.Require().NoError(err)

	s.repo = clicklog.NewRepository(pool)
}

func (s *ClickLogRepoSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		_ = s.container.Terminate(context.Background())
	}
}

func (s *ClickLogRepoSuite) SetupTest() {
	_, err := s.pool.Exec(context.Background(), "TRUNCATE click_log")
	s.Require().NoError(err)
}

// --- Test cases ---

func (s *ClickLogRepoSuite) TestBatchCreate_SingleRecord() {
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{
			ID:         "550e8400-e29b-41d4-a716-446655440001",
			ShortURLID: 1001,
			ShortCode:  "abc1234567",
			CreatorID:  "user_01",
			ReferralID: "ref_01",
			Referrer:   "https://google.com",
			UserAgent:  "Mozilla/5.0",
			IPAddress:  "1.2.3.4",
			IsBot:      false,
			CreatedAt:  time.Now().UTC(),
		},
	}

	err := s.repo.BatchCreate(ctx, logs)
	s.Require().NoError(err)

	// Verify the record exists.
	var count int
	s.Require().NoError(
		s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM click_log").Scan(&count),
	)
	s.Equal(1, count)
}

func (s *ClickLogRepoSuite) TestBatchCreate_MultipleRecords() {
	ctx := context.Background()

	logs := []*entity.ClickLog{
		{
			ID:         "550e8400-e29b-41d4-a716-446655440011",
			ShortURLID: 2001,
			ShortCode:  "multi00001",
			CreatorID:  "user_02",
			IsBot:      false,
			CreatedAt:  time.Now().UTC(),
		},
		{
			ID:         "550e8400-e29b-41d4-a716-446655440012",
			ShortURLID: 2001,
			ShortCode:  "multi00001",
			CreatorID:  "user_02",
			IsBot:      true,
			CreatedAt:  time.Now().UTC(),
		},
		{
			ID:         "550e8400-e29b-41d4-a716-446655440013",
			ShortURLID: 2001,
			ShortCode:  "multi00001",
			CreatorID:  "user_02",
			IsBot:      false,
			CreatedAt:  time.Now().UTC(),
		},
	}

	err := s.repo.BatchCreate(ctx, logs)
	s.Require().NoError(err)

	var count int
	s.Require().NoError(
		s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM click_log").Scan(&count),
	)
	s.Equal(3, count)
}

func (s *ClickLogRepoSuite) TestBatchCreate_EmptySlice_NoError() {
	ctx := context.Background()
	err := s.repo.BatchCreate(ctx, []*entity.ClickLog{})
	s.Require().NoError(err)
}

func TestClickLogRepo(t *testing.T) {
	suite.Run(t, new(ClickLogRepoSuite))
}
