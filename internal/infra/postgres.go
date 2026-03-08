package infra

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates and returns a pgxpool.Pool based on DatabaseConfig.
func NewPool(ctx context.Context, cfg DatabaseConfig) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns) //nolint:gosec
	// pgx v5 uses MinConns as an equivalent to MaxIdleConns.
	if cfg.MaxIdleConns > 0 {
		poolCfg.MinConns = int32(cfg.MaxIdleConns) //nolint:gosec
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	// Validate connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
