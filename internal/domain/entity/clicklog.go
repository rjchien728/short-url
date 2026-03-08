package entity

import "time"

// ClickLog records a single click event on a short URL.
// IPCountry is intentionally omitted — GeoIP lookup is not implemented in the POC.
type ClickLog struct {
	ID         string // UUID v7
	ShortURLID int64
	ShortCode  string
	CreatorID  string
	ReferralID string
	Referrer   string
	UserAgent  string
	IPAddress  string
	IsBot      bool
	CreatedAt  time.Time
}
