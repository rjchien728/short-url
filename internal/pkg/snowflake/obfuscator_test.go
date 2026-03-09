package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testSalt = int64(6364136223846793005)

// maxID53 is the maximum valid 53-bit ID value.
const maxID53 = int64(mask53)

// TestObfuscate_NoCollision verifies that 1000 consecutive IDs produce unique obfuscated values.
func TestObfuscate_NoCollision(t *testing.T) {
	seen := make(map[int64]bool, 1000)
	for id := int64(1); id <= 1000; id++ {
		out := Obfuscate(id, testSalt)
		assert.False(t, seen[out], "collision at id=%d: obfuscated=%d", id, out)
		seen[out] = true
	}
}

// TestObfuscate_Reversible verifies that Deobfuscate(Obfuscate(id)) == id for multiple values.
func TestObfuscate_Reversible(t *testing.T) {
	ids := []int64{0, 1, 42, 1000, 99999, maxID53}
	for _, id := range ids {
		obfuscated := Obfuscate(id, testSalt)
		recovered := Deobfuscate(obfuscated, testSalt)
		assert.Equal(t, id, recovered, "round-trip failed for id=%d", id)
	}
}

// TestObfuscate_Within53Bits verifies that the result never exceeds 53 bits.
func TestObfuscate_Within53Bits(t *testing.T) {
	for id := int64(0); id < 1000; id++ {
		out := Obfuscate(id, testSalt)
		assert.LessOrEqual(t, out, maxID53, "result exceeds 53 bits for id=%d", id)
		assert.GreaterOrEqual(t, out, int64(0), "result is negative for id=%d", id)
	}
}

// TestObfuscate_ZeroSalt verifies the function does not panic when salt is 0.
func TestObfuscate_ZeroSalt(t *testing.T) {
	assert.NotPanics(t, func() {
		out := Obfuscate(12345, 0)
		recovered := Deobfuscate(out, 0)
		assert.Equal(t, int64(12345), recovered)
	})
}
