package base58

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode_FixedLength(t *testing.T) {
	tests := []struct {
		desc string
		id   int64
	}{
		{"zero", 0},
		{"small number", 1},
		{"large 53-bit number", 1<<53 - 1},
		{"mid range", 12345678901},
		{"snowflake-like", 23612571684864},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			encoded := Encode(tt.id)
			assert.Len(t, encoded, fixedLen, "encoded length must be %d", fixedLen)
		})
	}
}

func TestEncode_Decode_Roundtrip(t *testing.T) {
	tests := []struct {
		desc string
		id   int64
	}{
		{"zero", 0},
		{"one", 1},
		{"small", 58},
		{"medium", 1000000},
		{"large 53-bit", 1<<53 - 1},
		{"snowflake-like", 23612571684864},
		{"another snowflake", 100000000000000},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			encoded := Encode(tt.id)
			decoded, err := Decode(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.id, decoded, "encode/decode roundtrip must be identity")
		})
	}
}

func TestDecode_InvalidInput(t *testing.T) {
	tests := []struct {
		desc  string
		input string
	}{
		{"empty string", ""},
		{"too long (11 chars)", "12345678901"},
		{"contains zero '0'", "000000000O"},
		{"contains lowercase 'l'", "1111111111l"},
		{"contains uppercase 'I'", "111111111I"},
		{"contains uppercase 'O'", "111111111O"},
		{"non-ascii character", "123456789\xff"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := Decode(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestIsValidShortCode(t *testing.T) {
	tests := []struct {
		desc     string
		input    string
		expected bool
	}{
		{"valid 10-char code", Encode(12345678), true},
		{"too short", "123456789", false},
		{"too long", "12345678901", false},
		{"contains invalid char O", "OOOOOOOOOO", false},
		{"contains invalid char 0", "0000000000", false},
		{"all ones (valid)", "1111111111", true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := IsValidShortCode(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEncode_OnlyUsesAlphabetChars(t *testing.T) {
	// All encoded characters must be in the alphabet set.
	for i := range 1000 {
		encoded := Encode(int64(i * 999983)) // use a prime step
		for _, c := range encoded {
			assert.Contains(t, alphabet, string(c),
				"character %q not in Base58 alphabet (id=%d)", c, int64(i*999983))
		}
	}
}
