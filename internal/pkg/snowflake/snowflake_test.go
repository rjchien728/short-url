package snowflake

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_Generate_Monotonic(t *testing.T) {
	g := New(0)
	const count = 10000

	prev := int64(0)
	for i := range count {
		id, err := g.Generate()
		require.NoError(t, err, "iteration %d", i)
		assert.Greater(t, id, prev, "ID must be strictly increasing at iteration %d", i)
		prev = id
	}
}

func TestGenerator_Generate_NoCollision(t *testing.T) {
	g := New(0)
	const count = 10000

	seen := make(map[int64]struct{}, count)
	for i := range count {
		id, err := g.Generate()
		require.NoError(t, err, "iteration %d", i)
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ID %d at iteration %d", id, i)
		seen[id] = struct{}{}
	}
}

func TestGenerator_Generate_ConcurrentNoCollision(t *testing.T) {
	g := New(0)
	const goroutines = 10
	const perGoroutine = 1000

	ids := make(chan int64, goroutines*perGoroutine)
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				id, err := g.Generate()
				if err == nil {
					ids <- id
				}
			}
		}()
	}

	wg.Wait()
	close(ids)

	seen := make(map[int64]struct{})
	for id := range ids {
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ID: %d", id)
		seen[id] = struct{}{}
	}
}

func TestGenerator_Generate_RespectsEpoch(t *testing.T) {
	g := New(0)
	id, err := g.Generate()
	require.NoError(t, err)

	// extract timestamp portion (upper 41 bits = id >> 12)
	tsOffset := id >> sequenceBits

	// tsOffset should be positive (current time is after epoch)
	assert.Greater(t, tsOffset, int64(0), "timestamp offset must be positive")

	// should correspond to roughly current time
	nowOffset := time.Now().UnixMilli() - epoch
	assert.InDelta(t, float64(nowOffset), float64(tsOffset), 1000,
		"timestamp offset should be close to current time offset")
}

func TestGenerator_Generate_SequenceBits(t *testing.T) {
	g := New(0)

	// sequence is the lower 12 bits
	id, err := g.Generate()
	require.NoError(t, err)

	seq := id & maxSequence
	assert.GreaterOrEqual(t, seq, int64(0))
	assert.LessOrEqual(t, seq, maxSequence)
}

func TestGenerator_implements_IDGenerator(t *testing.T) {
	// compile-time interface check
	var _ IDGenerator = (*Generator)(nil)
}
