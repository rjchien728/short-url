package service

import (
	"context"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// OGWorkerService processes Open Graph fetch tasks from the stream.
type OGWorkerService interface {
	ProcessTask(ctx context.Context, task *entity.OGFetchTask) error
}

// ClickWorkerService processes batches of click log events from the stream.
type ClickWorkerService interface {
	ProcessBatch(ctx context.Context, logs []*entity.ClickLog) error
}
