package redirect

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/singleflight"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/repository"
	"github.com/rjchien728/short-url/internal/pkg/logger"
	"github.com/rjchien728/short-url/internal/repository/urlcache"
)

// Service implements domain/service.RedirectService.
type Service struct {
	repo      repository.ShortURLRepository
	cache     repository.URLCache
	publisher repository.EventPublisher
	// sf deduplicates concurrent DB lookups for the same short code,
	// preventing cache-stampede when a popular URL's cache entry expires.
	sf singleflight.Group
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
//
// Lookup order:
//  1. Cache hit      → return immediately (TTL refreshed by cache layer)
//  2. Negative cache → return ErrNotFound without touching DB
//  3. Cache miss     → singleflight DB lookup → backfill cache
//
// Returns entity.ErrNotFound if the code does not exist,
// entity.ErrExpired if past expiry.
func (s *Service) Resolve(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	// 1 & 2: cache lookup (hit / negative / miss)
	url, err := s.cache.Get(ctx, shortCode)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			// negative cache hit — shortCode is known to be absent
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("redirect.Resolve: cache get: %w", err)
	}

	if url == nil {
		// 3: cache miss — use singleflight to deduplicate concurrent DB requests
		result, sfErr, _ := s.sf.Do(shortCode, func() (any, error) {
			return s.fetchAndCache(ctx, shortCode)
		})
		if sfErr != nil {
			return nil, sfErr
		}
		url = result.(*entity.ShortURL)
	}

	if url.IsExpired() {
		return nil, entity.ErrExpired
	}

	return url, nil
}

// fetchAndCache queries the DB and backfills the cache.
// On DB miss, it writes a negative-cache entry so subsequent requests don't hit the DB.
func (s *Service) fetchAndCache(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	url, err := s.repo.FindByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			// Write negative cache — failure is non-fatal.
			if setErr := s.cache.SetNotFound(ctx, shortCode); setErr != nil {
				logger.Warn(ctx, "failed to set negative cache", "short_code", shortCode, "error", setErr)
			}
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("redirect.Resolve: %w", err)
	}

	// Backfill cache — failure is non-fatal.
	if setErr := s.cache.Set(ctx, shortCode, url, urlcache.CacheTTL); setErr != nil {
		logger.Warn(ctx, "failed to backfill cache", "short_code", shortCode, "error", setErr)
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
