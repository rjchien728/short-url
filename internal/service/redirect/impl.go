package redirect

import (
	"context"
	"fmt"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/repository"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

// Service implements domain/service.RedirectService.
type Service struct {
	repo      repository.ShortURLRepository
	cache     repository.URLCache
	publisher repository.EventPublisher
}

// New creates a new RedirectService.
func New(
	repo repository.ShortURLRepository,
	cache repository.URLCache,
	publisher repository.EventPublisher,
) *Service {
	return &Service{
		repo:      repo,
		cache:     cache,
		publisher: publisher,
	}
}

// Resolve looks up a short URL by its short code.
// It checks the cache first; on miss it queries the DB and backfills the cache.
// Returns entity.ErrNotFound if the code does not exist, entity.ErrExpired if past expiry.
func (s *Service) Resolve(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	// cache lookup
	url, err := s.cache.Get(ctx, shortCode)
	if err != nil {
		return nil, fmt.Errorf("redirect.Resolve: cache get: %w", err)
	}

	if url == nil {
		// cache miss — fetch from DB
		url, err = s.repo.FindByShortCode(ctx, shortCode)
		if err != nil {
			return nil, fmt.Errorf("redirect.Resolve: %w", err)
		}

		// backfill cache; failure is non-fatal
		if setErr := s.cache.Set(ctx, shortCode, url); setErr != nil {
			logger.Warn(ctx, "failed to backfill cache", "short_code", shortCode, "error", setErr)
		}
	}

	if url.IsExpired() {
		return nil, entity.ErrExpired
	}

	return url, nil
}

// RecordClick publishes a click event to the stream.
// Failures are logged and do not block the redirect response.
func (s *Service) RecordClick(ctx context.Context, log *entity.ClickLog) error {
	if err := s.publisher.PublishClickEvent(ctx, log); err != nil {
		logger.Warn(ctx, "failed to publish click event", "short_code", log.ShortCode, "error", err)
	}
	return nil
}
