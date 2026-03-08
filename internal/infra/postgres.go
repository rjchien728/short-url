package infra

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool 建立並回傳一個 pgxpool.Pool，依照 DatabaseConfig 設定連線參數。
func NewPool(ctx context.Context, cfg DatabaseConfig) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns) //nolint:gosec
	// pgx v5 沒有 MaxIdleConns，以 MinConns 對應（確保至少保有 idle 連線）
	if cfg.MaxIdleConns > 0 {
		poolCfg.MinConns = int32(cfg.MaxIdleConns) //nolint:gosec
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	// 驗證連線是否可用
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
