CREATE TABLE IF NOT EXISTS short_url (
    id BIGINT PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL UNIQUE,
    long_url TEXT NOT NULL,
    creator_id VARCHAR(50) NOT NULL,
    og_metadata JSONB,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_short_url_short_code ON short_url(short_code);
