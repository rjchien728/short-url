package clicklog

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

// Repository implements domain/repository.ClickLogRepository using pgx.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new ClickLog repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// BatchCreate inserts multiple ClickLog records in a single statement.
// It builds a multi-row INSERT dynamically to minimise round-trips.
func (r *Repository) BatchCreate(ctx context.Context, logs []*entity.ClickLog) error {
	if len(logs) == 0 {
		return nil
	}

	const cols = 10 // number of columns per row
	args := make([]any, 0, len(logs)*cols)
	placeholders := make([]string, 0, len(logs))

	for i, log := range logs {
		base := i * cols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10,
		))
		args = append(args,
			log.ID,
			log.ShortURLID,
			log.ShortCode,
			log.CreatorID,
			log.ReferralID,
			log.Referrer,
			log.UserAgent,
			log.IPAddress,
			log.IsBot,
			log.CountryCode,
		)
	}

	query := `INSERT INTO click_log
		(id, short_url_id, short_code, creator_id, referral_id, referrer, user_agent, ip_address, is_bot, country_code)
		VALUES ` + strings.Join(placeholders, ", ")

	_, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("clicklog.BatchCreate: %w", err)
	}
	return nil
}
