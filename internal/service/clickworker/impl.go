package clickworker

import (
	"context"
	"fmt"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/domain/repository"
	"github.com/rjchien728/short-url/internal/pkg/geoip"
	"github.com/rjchien728/short-url/internal/pkg/logger"
)

// Service implements domain/service.ClickWorkerService.
type Service struct {
	repo  repository.ClickLogRepository
	geoIP geoip.Resolver
}

// New creates a new ClickWorkerService.
func New(repo repository.ClickLogRepository, geoIP geoip.Resolver) *Service {
	return &Service{repo: repo, geoIP: geoIP}
}

// ProcessBatch persists a batch of click log events to the database.
// It enriches each log with a country code via GeoIP lookup before writing.
// On error the caller (consumer) should not XACK, allowing the PEL to retry delivery.
func (s *Service) ProcessBatch(ctx context.Context, logs []*entity.ClickLog) error {
	for _, log := range logs {
		country, err := s.geoIP.LookupCountry(log.IPAddress)
		if err != nil {
			logger.Warn(ctx, "geoip lookup failed", "ip", log.IPAddress, "err", err)
			// CountryCode stays nil; continue writing the rest of the fields.
			continue
		}
		if country != "" {
			log.CountryCode = &country
		}
	}

	if err := s.repo.BatchCreate(ctx, logs); err != nil {
		return fmt.Errorf("clickworker.ProcessBatch: %w", err)
	}
	return nil
}
