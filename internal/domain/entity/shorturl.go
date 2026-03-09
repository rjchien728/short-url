package entity

import "time"

// ShortURL represents a short URL record stored in the database.
type ShortURL struct {
	ID         int64
	ShortCode  string
	LongURL    string
	CreatorID  string
	OGMetadata *OGMetadata
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// IsExpired reports whether the short URL has passed its expiry time.
// URLs with no expiry (ExpiresAt == nil) never expire.
func (s *ShortURL) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}

// OGMetadata holds the Open Graph metadata scraped from the destination URL.
type OGMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"site_name"`
	FetchFailed bool   `json:"fetch_failed"`
}

// OGFetchTask is the event payload published to stream:og-fetch.
// RetryCount tracks how many times the fetch has been attempted (0 = first attempt).
type OGFetchTask struct {
	ShortURLID int64
	ShortCode  string
	LongURL    string
	RetryCount int
}
