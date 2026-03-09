package entity

import "time"

// ClickLog records a single click event on a short URL.
type ClickLog struct {
	ID          string // UUID v7
	ShortURLID  int64
	ShortCode   string
	CreatorID   string
	ReferralID  string
	Referrer    string
	UserAgent   string
	IPAddress   string
	IsBot       bool
	CountryCode *string // ISO 3166-1 alpha-2, e.g. "TW". NULL if lookup failed or IP is private.
	CreatedAt   time.Time
}
