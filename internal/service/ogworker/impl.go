package ogworker

import (
	"context"
	"fmt"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/gateway"
	"github.com/rjchien728/short-url/internal/domain/repository"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

const maxRetry = 3

// Service implements domain/service.OGWorkerService.
// On fetch failure it re-enqueues the task with an incremented RetryCount instead of sleeping.
// Once RetryCount reaches maxRetry, the URL is marked as FetchFailed.
type Service struct {
	repo      repository.ShortURLRepository
	cache     repository.URLCache
	fetcher   gateway.OGFetcher
	publisher repository.EventPublisher
}

// New creates a new OGWorkerService.
func New(
	repo repository.ShortURLRepository,
	cache repository.URLCache,
	fetcher gateway.OGFetcher,
	publisher repository.EventPublisher,
) *Service {
	return &Service{
		repo:      repo,
		cache:     cache,
		fetcher:   fetcher,
		publisher: publisher,
	}
}

// ProcessTask fetches OG metadata for the given task's URL.
// On failure it re-enqueues the task (RetryCount + 1) until maxRetry is reached,
// then marks the short URL as fetch-failed. This avoids blocking the worker goroutine with sleeps.
// After any DB write (success or max-retry), the cache entry is invalidated so the next
// request reads fresh OG data instead of the stale nil version.
func (s *Service) ProcessTask(ctx context.Context, task *entity.OGFetchTask) error {
	metadata, err := s.fetcher.Fetch(ctx, task.LongURL)
	if err != nil {
		if task.RetryCount >= maxRetry {
			// all retries exhausted — mark as permanently failed and XACK the message
			if updateErr := s.repo.UpdateOGMetadata(ctx, task.ShortURLID, &entity.OGMetadata{FetchFailed: true}); updateErr != nil {
				return fmt.Errorf("ogworker.ProcessTask: mark fetch failed: %w", updateErr)
			}
			s.invalidateCache(ctx, task.ShortCode)
			return nil
		}

		// re-enqueue with incremented retry count; goroutine is free immediately
		if pubErr := s.publisher.PublishOGFetchTask(ctx, &entity.OGFetchTask{
			ShortURLID: task.ShortURLID,
			ShortCode:  task.ShortCode,
			LongURL:    task.LongURL,
			RetryCount: task.RetryCount + 1,
		}); pubErr != nil {
			return fmt.Errorf("ogworker.ProcessTask: re-enqueue: %w", pubErr)
		}
		return nil
	}

	// fetch succeeded — persist metadata
	if updateErr := s.repo.UpdateOGMetadata(ctx, task.ShortURLID, metadata); updateErr != nil {
		return fmt.Errorf("ogworker.ProcessTask: update og metadata: %w", updateErr)
	}
	s.invalidateCache(ctx, task.ShortCode)
	return nil
}

// invalidateCache removes the cached ShortURL entry so the next request reads fresh OG data.
// Cache is best-effort; failure is logged as a warning and does not affect correctness
// (the cache will expire naturally within its TTL).
func (s *Service) invalidateCache(ctx context.Context, shortCode string) {
	if err := s.cache.Delete(ctx, shortCode); err != nil {
		logger.Warn(ctx, "ogworker: failed to invalidate cache after OG update", "short_code", shortCode, "error", err)
	}
}
