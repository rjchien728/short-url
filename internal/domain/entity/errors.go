package entity

import "errors"

// Sentinel errors for domain-level error handling.
// Handlers use errors.Is() to map these to HTTP status codes.
var (
	ErrNotFound = errors.New("short url not found")
	ErrExpired  = errors.New("short url expired")
)
