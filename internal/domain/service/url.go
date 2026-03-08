//go:generate mockgen -destination=../../mock/mock_service.go -package=mock github.com/rjchien728/short-url/internal/domain/service URLService,RedirectService

package service

import (
	"context"
	"time"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// URLService handles the creation of short URLs.
type URLService interface {
	Create(ctx context.Context, req CreateURLRequest) (*entity.ShortURL, error)
}

// CreateURLRequest is the input for URLService.Create.
type CreateURLRequest struct {
	LongURL   string
	CreatorID string
	ExpiresAt *time.Time
}
