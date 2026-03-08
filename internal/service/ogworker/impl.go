package ogworker

import (
	"context"
	"fmt"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/gateway"
	"github.com/rjchien728/short-url/internal/domain/repository"
)

const maxRetry = 3

// Service implements domain/service.OGWorkerService.
// On fetch failure it re-enqueues the task with an incremented RetryCount instead of sleeping.
// Once RetryCount reaches maxRetry, the URL is marked as FetchFailed.
type Service struct {
	repo      repository.ShortURLRepository
	fetcher   gateway.OGFetcher
	publisher repository.EventPublisher
}

// New creates a new OGWorkerService.
func New(
	repo repository.ShortURLRepository,
	fetcher gateway.OGFetcher,
	publisher repository.EventPublisher,
) *Service {
	return &Service{
		repo:      repo,
		fetcher:   fetcher,
		publisher: publisher,
	}
}

// ProcessTask fetches OG metadata for the given task's URL.
// On failure it re-enqueues the task (RetryCount + 1) until maxRetry is reached,
// then marks the short URL as fetch-failed. This avoids blocking the worker goroutine with sleeps.
func (s *Service) ProcessTask(ctx context.Context, task *entity.OGFetchTask) error {
	metadata, err := s.fetcher.Fetch(ctx, task.LongURL)
	if err != nil {
		if task.RetryCount >= maxRetry {
			// all retries exhausted — mark as permanently failed and XACK the message
			if updateErr := s.repo.UpdateOGMetadata(ctx, task.ShortURLID, &entity.OGMetadata{FetchFailed: true}); updateErr != nil {
				return fmt.Errorf("ogworker.ProcessTask: mark fetch failed: %w", updateErr)
			}
			return nil
		}

		// re-enqueue with incremented retry count; goroutine is free immediately
		if pubErr := s.publisher.PublishOGFetchTask(ctx, &entity.OGFetchTask{
			ShortURLID: task.ShortURLID,
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
	return nil
}
