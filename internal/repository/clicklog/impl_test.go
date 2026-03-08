package clicklog_test

// Integration tests for ClickLog repository.
//
// Prerequisites: a running PostgreSQL instance with migrations applied.
// Set DB_DSN environment variable (or use .env file) before running.
//
// Run with:
//   make test-integration
// or:
//   DB_DSN=postgres://... go test ./internal/repository/clicklog/...

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/repository/clicklog"
)

type ClickLogRepoSuite struct {
	suite.Suite
	pool *pgxpool.Pool
	repo *clicklog.Repository
}

func (s *ClickLogRepoSuite) SetupSuite() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		s.T().Skip("DB_DSN not set — skipping integration tests")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	s.Require().NoError(err, "failed to connect to database")
	s.pool = pool
	s.repo = clicklog.NewRepository(pool)
}

func (s *ClickLogRepoSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
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
