//go:generate mockgen -destination=../../mock/mock_gateway.go -package=mock github.com/rjchien728/short-url/internal/domain/gateway OGFetcher
package gateway

import (
	"context"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// OGFetcher retrieves Open Graph metadata from a target URL.
type OGFetcher interface {
	Fetch(ctx context.Context, url string) (*entity.OGMetadata, error)
}
