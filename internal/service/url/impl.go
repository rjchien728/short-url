package urlsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/repository"
	domainservice "github.com/rjchien728/short-url/internal/domain/service"
	"github.com/rjchien728/short-url/internal/pkg/base58"
	"github.com/rjchien728/short-url/internal/pkg/logger"
	"github.com/rjchien728/short-url/internal/pkg/snowflake"
)

const maxCreateRetry = 3

// Service implements domain/service.URLService.
type Service struct {
	repo      repository.ShortURLRepository
	cache     repository.URLCache
	publisher repository.EventPublisher
	idGen     snowflake.IDGenerator
}

// New creates a new URLService.
func New(
	repo repository.ShortURLRepository,
	cache repository.URLCache,
	publisher repository.EventPublisher,
	idGen snowflake.IDGenerator,
) *Service {
	return &Service{
		repo:      repo,
		cache:     cache,
		publisher: publisher,
		idGen:     idGen,
	}
}

// Create generates a short code and persists the short URL.
// On unique constraint violation (short_code collision), it retries up to maxCreateRetry times.
// After a successful create, it publishes an OG fetch task; publisher failure only logs a warning.
func (s *Service) Create(ctx context.Context, req domainservice.CreateURLRequest) (*entity.ShortURL, error) {
	var (
		url *entity.ShortURL
		err error
	)

	for i := range maxCreateRetry {
		id, genErr := s.idGen.Generate()
		if genErr != nil {
			return nil, fmt.Errorf("urlsvc.Create: generate id: %w", genErr)
		}

		shortCode := base58.Encode(id)
		url = &entity.ShortURL{
			ID:        id,
			ShortCode: shortCode,
			LongURL:   req.LongURL,
			CreatorID: req.CreatorID,
			ExpiresAt: req.ExpiresAt,
			CreatedAt: time.Now().UTC(),
		}

		err = s.repo.Create(ctx, url)
		if err == nil {
			break // success
		}

		// retry only on unique constraint violation (short_code collision)
		if !isUniqueViolation(err) {
			return nil, fmt.Errorf("urlsvc.Create: %w", err)
		}

		logger.Warn(ctx, "short_code collision, retrying", "attempt", i+1)
	}

	if err != nil {
		return nil, fmt.Errorf("urlsvc.Create: max retries exceeded: %w", err)
	}

	// publish OG fetch task — failure is non-fatal; OG metadata is best-effort
	if pubErr := s.publisher.PublishOGFetchTask(ctx, &entity.OGFetchTask{
		ShortURLID: url.ID,
		ShortCode:  url.ShortCode,
		LongURL:    url.LongURL,
	}); pubErr != nil {
		logger.Warn(ctx, "failed to publish OG fetch task", "short_code", url.ShortCode, "error", pubErr)
	}

	return url, nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique constraint violation (code 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
