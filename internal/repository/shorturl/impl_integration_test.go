package shorturl_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/repository/shorturl"
)

type ShortURLRepoSuite struct {
	suite.Suite
	container *tcpostgres.PostgresContainer
	repo      *shorturl.Repository
	pool      *pgxpool.Pool
}

func (s *ShortURLRepoSuite) SetupSuite() {
	ctx := context.Background()

	// Read migration SQL from project root.
	createShortURL, err := os.ReadFile("../../../migrations/000001_create_short_url.up.sql")
	s.Require().NoError(err)

	// Start a temporary PostgreSQL 17 container.
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

	// Connect pool.
	pool, err := pgxpool.New(ctx, dsn)
	s.Require().NoError(err)
	s.pool = pool

	// Run migration to create the short_url table.
	_, err = pool.Exec(ctx, string(createShortURL))
	s.Require().NoError(err)

	s.repo = shorturl.NewRepository(pool)
}

func (s *ShortURLRepoSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		_ = s.container.Terminate(context.Background())
	}
}

func (s *ShortURLRepoSuite) SetupTest() {
	// Clean table before each test case to keep tests independent.
	_, err := s.pool.Exec(context.Background(), "TRUNCATE short_url")
	s.Require().NoError(err)
}

// --- Test cases ---

func (s *ShortURLRepoSuite) TestCreate_HappyPath() {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	url := &entity.ShortURL{
		ID:        1001,
		ShortCode: "abc1234567",
		LongURL:   "https://example.com/path",
		CreatorID: "user_01",
		ExpiresAt: nil,
		CreatedAt: now,
	}

	err := s.repo.Create(ctx, url)
	s.Require().NoError(err)
}

func (s *ShortURLRepoSuite) TestCreate_DuplicateShortCode_ReturnsError() {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	url := &entity.ShortURL{
		ID:        1002,
		ShortCode: "dup0000001",
		LongURL:   "https://example.com",
		CreatorID: "user_01",
		CreatedAt: now,
	}
	s.Require().NoError(s.repo.Create(ctx, url))

	// Same short_code, different ID — unique constraint violation.
	url2 := &entity.ShortURL{
		ID:        1003,
		ShortCode: "dup0000001",
		LongURL:   "https://example.com/other",
		CreatorID: "user_01",
		CreatedAt: now,
	}
	err := s.repo.Create(ctx, url2)
	s.Require().Error(err)
}

func (s *ShortURLRepoSuite) TestFindByShortCode_Found() {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	url := &entity.ShortURL{
		ID:        2001,
		ShortCode: "find000001",
		LongURL:   "https://example.com/find",
		CreatorID: "user_02",
		CreatedAt: now,
	}
	s.Require().NoError(s.repo.Create(ctx, url))

	got, err := s.repo.FindByShortCode(ctx, "find000001")
	s.Require().NoError(err)
	s.Equal(url.ID, got.ID)
	s.Equal(url.ShortCode, got.ShortCode)
	s.Equal(url.LongURL, got.LongURL)
	s.Equal(url.CreatorID, got.CreatorID)
	s.Nil(got.OGMetadata)
	s.Nil(got.ExpiresAt)
}

func (s *ShortURLRepoSuite) TestFindByShortCode_NotFound() {
	ctx := context.Background()

	_, err := s.repo.FindByShortCode(ctx, "notexists0")
	s.Require().ErrorIs(err, entity.ErrNotFound)
}

func (s *ShortURLRepoSuite) TestUpdateOGMetadata_Success() {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	url := &entity.ShortURL{
		ID:        3001,
		ShortCode: "ogupdate01",
		LongURL:   "https://example.com/og",
		CreatorID: "user_03",
		CreatedAt: now,
	}
	s.Require().NoError(s.repo.Create(ctx, url))

	og := &entity.OGMetadata{
		Title:       "Test Title",
		Description: "Test Description",
		Image:       "https://example.com/img.png",
		SiteName:    "Example",
		FetchFailed: false,
	}
	err := s.repo.UpdateOGMetadata(ctx, url.ID, og)
	s.Require().NoError(err)

	got, err := s.repo.FindByShortCode(ctx, "ogupdate01")
	s.Require().NoError(err)
	s.Require().NotNil(got.OGMetadata)
	s.Equal(og.Title, got.OGMetadata.Title)
	s.Equal(og.Description, got.OGMetadata.Description)
	s.Equal(og.Image, got.OGMetadata.Image)
	s.Equal(og.SiteName, got.OGMetadata.SiteName)
	s.False(got.OGMetadata.FetchFailed)
}

func (s *ShortURLRepoSuite) TestUpdateOGMetadata_FetchFailed() {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	url := &entity.ShortURL{
		ID:        3002,
		ShortCode: "ogfailed01",
		LongURL:   "https://example.com/og-fail",
		CreatorID: "user_03",
		CreatedAt: now,
	}
	s.Require().NoError(s.repo.Create(ctx, url))

	og := &entity.OGMetadata{FetchFailed: true}
	err := s.repo.UpdateOGMetadata(ctx, url.ID, og)
	s.Require().NoError(err)

	got, err := s.repo.FindByShortCode(ctx, "ogfailed01")
	s.Require().NoError(err)
	s.Require().NotNil(got.OGMetadata)
	s.True(got.OGMetadata.FetchFailed)
}

func TestShortURLRepo(t *testing.T) {
	suite.Run(t, new(ShortURLRepoSuite))
}
