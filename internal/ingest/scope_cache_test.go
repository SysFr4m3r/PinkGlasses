package ingest

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// The cache is shared by every /agent/v1/results request. A worker dispatches
// each job in its own goroutine and each posts its own results, so a single
// worker at the default concurrency already reaches it from eight goroutines.
//
// This test exists because the map behind it was once unguarded, which is not
// a corruption bug but a fatal one: the runtime throws `concurrent map writes`
// and the gateway process dies, taking every worker's control channel with it.
//
// It guards the locking, not the original bug — take the RWMutex out of
// scopeCache and this fails under -race. CI runs the suite with -race, which
// is the point: without the detector the same code may well pass by luck.
func TestScopeCacheSurvivesConcurrentUse(t *testing.T) {
	t.Parallel()
	c := newScopeCache()

	// Distinct runs, so goroutines write different keys: that is the
	// read-while-another-writes collision, which is fatal too.
	const runs = 64
	ids := make([]uuid.UUID, runs)
	scopes := make([]uuid.UUID, runs)
	for i := range ids {
		ids[i], scopes[i] = uuid.New(), uuid.New()
	}

	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				c.put(ids[i], scopes[i])
				// Read a neighbour's key, not our own, so reads and writes
				// genuinely overlap rather than each goroutine owning a lane.
				if got, ok := c.get(ids[(i+1)%runs]); ok && got == uuid.Nil {
					t.Errorf("cached a nil scope id for run %s", ids[(i+1)%runs])
					return
				}
			}
		}(i)
	}
	wg.Wait()

	for i := range ids {
		got, ok := c.get(ids[i])
		if !ok {
			t.Fatalf("run %s fell out of the cache", ids[i])
		}
		if got != scopes[i] {
			t.Fatalf("run %s cached scope %s, want %s", ids[i], got, scopes[i])
		}
	}
}

// The gateway is never told a run has finished, so the cache would otherwise
// grow one entry per run for the life of the process.
func TestScopeCacheStaysBounded(t *testing.T) {
	t.Parallel()
	c := newScopeCache()

	for i := 0; i < maxScopeCache*2+1; i++ {
		c.put(uuid.New(), uuid.New())
	}

	c.mu.RLock()
	n := len(c.m)
	c.mu.RUnlock()
	if n > maxScopeCache {
		t.Fatalf("cache holds %d entries, over the %d ceiling", n, maxScopeCache)
	}

	// Clearing must not break the cache: the entry written after a reset is
	// still the one that comes back.
	run, scope := uuid.New(), uuid.New()
	c.put(run, scope)
	if got, ok := c.get(run); !ok || got != scope {
		t.Fatalf("after eviction got (%s, %v), want (%s, true)", got, ok, scope)
	}
}
