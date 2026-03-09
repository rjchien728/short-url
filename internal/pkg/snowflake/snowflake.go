//go:generate mockgen -destination=../../../internal/mock/mock_snowflake.go -package=mock github.com/rjchien728/short-url/internal/pkg/snowflake IDGenerator
package snowflake

import (
	"fmt"
	"sync"
	"time"

	"github.com/rjchien728/short-url/internal/pkg/base58"
)

const (
	// epoch is 2026-01-01T00:00:00Z in Unix milliseconds.
	epoch int64 = 1767225600000

	// sequenceBits is the number of bits allocated for the per-ms sequence number.
	sequenceBits uint = 12

	// maxSequence is the maximum value of the 12-bit sequence (4095).
	maxSequence int64 = (1 << sequenceBits) - 1
)

// IDGenerator defines the interface for generating unique int64 IDs
// and encoding them to short codes.
type IDGenerator interface {
	// Generate produces a unique, monotonically increasing int64 ID for use as a DB primary key.
	Generate() (int64, error)
	// ShortCode encodes a raw ID into an obfuscated Base58 short code.
	// The obfuscation salt is held internally; callers do not need to know it.
	ShortCode(id int64) string
}

// Generator is a simplified Snowflake ID generator.
// Bit layout (53 bits total, fits JS Number.MAX_SAFE_INTEGER):
//
//	[41-bit ms timestamp since epoch | 12-bit per-ms sequence]
type Generator struct {
	mu       sync.Mutex
	lastMS   int64 // last millisecond timestamp used
	sequence int64 // current sequence number within the same millisecond
	salt     int64 // salt for ID obfuscation; kept internal, not exposed to callers
}

// New creates a new Snowflake Generator with the given obfuscation salt.
// The salt should be a secret random int64 set per deployment.
func New(salt int64) *Generator {
	return &Generator{salt: salt}
}

// ShortCode obfuscates id and encodes it to a 10-character Base58 string.
// The raw id (DB primary key) is never modified; only the display code is obfuscated.
func (g *Generator) ShortCode(id int64) string {
	return base58.Encode(Obfuscate(id, g.salt))
}

// Generate produces a unique, monotonically increasing int64 ID.
// If the per-ms sequence overflows (> 4095), it spin-waits until the next millisecond.
func (g *Generator) Generate() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := currentMS()

	if now == g.lastMS {
		// same millisecond: increment sequence
		g.sequence++
		if g.sequence > maxSequence {
			// sequence overflow: spin-wait for next millisecond
			for now <= g.lastMS {
				now = currentMS()
			}
			// new millisecond after spin-wait
			g.sequence = 0
			g.lastMS = now
		}
	} else if now > g.lastMS {
		// new millisecond: reset sequence
		g.sequence = 0
		g.lastMS = now
	} else {
		// clock moved backwards
		return 0, fmt.Errorf("clock moved backwards: last=%d now=%d", g.lastMS, now)
	}

	ts := now - epoch
	if ts < 0 {
		return 0, fmt.Errorf("current time is before snowflake epoch")
	}

	id := (ts << sequenceBits) | g.sequence
	return id, nil
}

// currentMS returns the current Unix timestamp in milliseconds.
func currentMS() int64 {
	return time.Now().UnixMilli()
}
