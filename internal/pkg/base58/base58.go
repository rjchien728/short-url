package base58

import (
	"fmt"
	"strings"
)

const (
	// alphabet is the Base58 character set (Bitcoin-style, no 0/O/I/l).
	alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// base is the length of the alphabet (58).
	base = int64(len(alphabet))

	// fixedLen is the fixed output length for encoded short codes.
	fixedLen = 10
)

// alphabetIdx maps each Base58 character back to its index for decoding.
var alphabetIdx [128]int8

func init() {
	// initialise all to -1 (invalid)
	for i := range alphabetIdx {
		alphabetIdx[i] = -1
	}
	for i, c := range alphabet {
		alphabetIdx[c] = int8(i)
	}
}

// Encode converts a non-negative int64 into a 10-character Base58 string.
// The output is left-padded with the first alphabet character ('1') to reach fixedLen.
func Encode(id int64) string {
	if id < 0 {
		// treat negative as unsigned by using absolute value
		id = -id
	}

	buf := make([]byte, fixedLen)
	// fill from right to left
	pos := fixedLen - 1
	for id > 0 {
		buf[pos] = alphabet[id%base]
		id /= base
		pos--
	}
	// left-pad remaining positions with the '1' character (index 0)
	for pos >= 0 {
		buf[pos] = alphabet[0]
		pos--
	}
	return string(buf)
}

// Decode converts a Base58-encoded string back to an int64.
// Returns an error for invalid characters or strings that exceed 10 characters.
func Decode(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("base58: empty input")
	}
	if len(s) > fixedLen {
		return 0, fmt.Errorf("base58: input too long (%d > %d)", len(s), fixedLen)
	}

	var result int64
	for _, c := range s {
		if c >= 128 || alphabetIdx[c] == -1 {
			return 0, fmt.Errorf("base58: invalid character %q", c)
		}
		result = result*base + int64(alphabetIdx[c])
	}
	return result, nil
}

// IsValidShortCode reports whether s is a valid 10-character Base58 string.
func IsValidShortCode(s string) bool {
	if len(s) != fixedLen {
		return false
	}
	return strings.IndexFunc(s, func(c rune) bool {
		return c >= 128 || alphabetIdx[c] == -1
	}) == -1
}
