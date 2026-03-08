package clickworker

import (
	"context"
	"fmt"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/repository"
)

// Service implements domain/service.ClickWorkerService.
type Service struct {
	repo repository.ClickLogRepository
}

// New creates a new ClickWorkerService.
func New(repo repository.ClickLogRepository) *Service {
	return &Service{repo: repo}
}

// ProcessBatch persists a batch of click log events to the database.
// On error the caller (consumer) should not XACK, allowing the PEL to retry delivery.
func (s *Service) ProcessBatch(ctx context.Context, logs []*entity.ClickLog) error {
	if err := s.repo.BatchCreate(ctx, logs); err != nil {
		return fmt.Errorf("clickworker.ProcessBatch: %w", err)
	}
	return nil
}
