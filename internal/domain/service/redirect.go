package service

import (
	"context"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// RedirectService resolves short codes and records click events.
type RedirectService interface {
	Resolve(ctx context.Context, shortCode string) (*entity.ShortURL, error)
	RecordClick(ctx context.Context, log *entity.ClickLog) error
}
