package geoip

import (
	"fmt"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// Resolver looks up country information from an IP address.
type Resolver interface {
	// LookupCountry returns the ISO 3166-1 alpha-2 country code (e.g. "TW") for the given IP.
	// Returns ("", nil) when the IP is private, loopback, or has no country record.
	// Returns ("", err) on MMDB read errors.
	LookupCountry(ip string) (string, error)
}

// Reader wraps a MaxMind GeoLite2-Country MMDB file.
type Reader struct {
	db *geoip2.Reader
}

// NewReader opens a GeoLite2-Country MMDB file at the given path.
func NewReader(path string) (*Reader, error) {
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoip.NewReader: %w", err)
	}
	return &Reader{db: db}, nil
}

// Close releases the underlying MMDB file handle.
func (r *Reader) Close() error {
	return r.db.Close()
}

// LookupCountry returns the ISO 3166-1 alpha-2 country code for the given IP string.
// Private/loopback addresses return ("", nil) since they have no meaningful country.
func (r *Reader) LookupCountry(ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", nil
	}

	// Private and loopback addresses have no country.
	if parsed.IsPrivate() || parsed.IsLoopback() {
		return "", nil
	}

	record, err := r.db.Country(parsed)
	if err != nil {
		return "", fmt.Errorf("geoip.LookupCountry: %w", err)
	}

	return record.Country.IsoCode, nil
}
