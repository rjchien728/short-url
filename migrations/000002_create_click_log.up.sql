CREATE TABLE IF NOT EXISTS click_log (
    id UUID PRIMARY KEY,
    short_url_id BIGINT NOT NULL,
    short_code VARCHAR(10) NOT NULL,
    creator_id VARCHAR(50) NOT NULL,
    referral_id VARCHAR(50),
    referrer TEXT,
    user_agent TEXT,
    ip_address VARCHAR(45),
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_click_log_short_url_id ON click_log(short_url_id);
CREATE INDEX idx_click_log_short_code ON click_log(short_code);
CREATE INDEX idx_click_log_creator_id ON click_log(creator_id);
