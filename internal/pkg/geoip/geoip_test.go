package geoip_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rjchien728/short-url/internal/pkg/geoip"
)

// mmdbPath returns the path to the test MMDB file.
// It looks for GEOIP_DB_PATH env var first, then falls back to the local dev path.
func mmdbPath() string {
	if p := os.Getenv("GEOIP_DB_PATH"); p != "" {
		return p
	}
	return "../../../../data/GeoLite2-Country.mmdb"
}

func newTestReader(t *testing.T) *geoip.Reader {
	t.Helper()
	r, err := geoip.NewReader(mmdbPath())
	if err != nil {
		t.Skipf("skipping geoip test: cannot open MMDB (%v)", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestReader_LookupCountry(t *testing.T) {
	r := newTestReader(t)

	tests := []struct {
		desc            string
		ip              string
		expectedCountry string
		expectErr       bool
	}{
		{
			desc:            "public IP returns country code",
			ip:              "8.8.8.8", // Google DNS, resolves to US
			expectedCountry: "US",
		},
		{
			desc:            "loopback IP returns empty string",
			ip:              "127.0.0.1",
			expectedCountry: "",
		},
		{
			desc:            "private IP (RFC 1918) returns empty string",
			ip:              "192.168.1.1",
			expectedCountry: "",
		},
		{
			desc:            "private IPv6 loopback returns empty string",
			ip:              "::1",
			expectedCountry: "",
		},
		{
			desc:            "invalid IP string returns empty string",
			ip:              "not-an-ip",
			expectedCountry: "",
		},
		{
			desc:            "empty IP string returns empty string",
			ip:              "",
			expectedCountry: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			country, err := r.LookupCountry(tt.ip)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCountry, country)
		})
	}
}
