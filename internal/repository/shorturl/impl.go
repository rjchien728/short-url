package shorturl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rjchien728/short-url/internal/domain/entity"
)

const (
	createSQL = `
		INSERT INTO short_url (id, short_code, long_url, creator_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	findByShortCodeSQL = `
		SELECT id, short_code, long_url, creator_id, og_metadata, expires_at, created_at
		FROM short_url
		WHERE short_code = $1`

	updateOGMetadataSQL = `
		UPDATE short_url SET og_metadata = $1 WHERE id = $2`
)

// Repository implements domain/repository.ShortURLRepository using pgx.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new ShortURL repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new ShortURL record into the database.
func (r *Repository) Create(ctx context.Context, url *entity.ShortURL) error {
	_, err := r.pool.Exec(ctx, createSQL,
		url.ID,
		url.ShortCode,
		url.LongURL,
		url.CreatorID,
		url.ExpiresAt,
		url.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("shorturl.Create: %w", err)
	}
	return nil
}

// FindByShortCode retrieves a ShortURL by its short code.
// Returns entity.ErrNotFound when no record matches.
func (r *Repository) FindByShortCode(ctx context.Context, shortCode string) (*entity.ShortURL, error) {
	row := r.pool.QueryRow(ctx, findByShortCodeSQL, shortCode)

	var (
		url    entity.ShortURL
		ogJSON []byte
	)

	err := row.Scan(
		&url.ID,
		&url.ShortCode,
		&url.LongURL,
		&url.CreatorID,
		&ogJSON,
		&url.ExpiresAt,
		&url.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("shorturl.FindByShortCode: %w", err)
	}

	// Parse JSONB og_metadata if present.
	if ogJSON != nil {
		var og entity.OGMetadata
		if err := json.Unmarshal(ogJSON, &og); err != nil {
			return nil, fmt.Errorf("shorturl.FindByShortCode: unmarshal og_metadata: %w", err)
		}
		url.OGMetadata = &og
	}

	return &url, nil
}

// UpdateOGMetadata writes the OG metadata JSONB for the given ShortURL ID.
func (r *Repository) UpdateOGMetadata(ctx context.Context, id int64, metadata *entity.OGMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("shorturl.UpdateOGMetadata: marshal metadata: %w", err)
	}

	_, err = r.pool.Exec(ctx, updateOGMetadataSQL, data, id)
	if err != nil {
		return fmt.Errorf("shorturl.UpdateOGMetadata: %w", err)
	}
	return nil
}
