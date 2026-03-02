package main

import (
	"fmt"
	"sync/atomic"
)

// Metrics tracks cache activity using lock-free atomic counters, so hot-path
// operations like Get and Set add virtually zero overhead.
type Metrics struct {
	hits      atomic.Int64 // Get() returned a live value
	misses    atomic.Int64 // Get() returned nothing (absent or TTL-expired)
	sets      atomic.Int64 // Set() calls
	deletes   atomic.Int64 // explicit Delete() calls
	evictions atomic.Int64 // items removed by policy or TTL expiry
}

// Snapshot is an immutable, point-in-time copy of the cache metrics.
type Snapshot struct {
	Hits      int64
	Misses    int64
	Sets      int64
	Deletes   int64
	Evictions int64
	HitRate   float64 // Hits / (Hits + Misses); 0 when no lookups yet
	MissRate  float64 // Misses / (Hits + Misses); 0 when no lookups yet
}

// String formats the snapshot for convenient logging.
func (s Snapshot) String() string {
	return fmt.Sprintf(
		"Hits=%-6d  Misses=%-6d  HitRate=%5.1f%%  Sets=%-6d  Deletes=%-6d  Evictions=%-6d",
		s.Hits, s.Misses, s.HitRate*100,
		s.Sets, s.Deletes, s.Evictions,
	)
}

// snapshot reads all counters atomically and computes the derived rates.
func (m *Metrics) snapshot() Snapshot {
	hits := m.hits.Load()
	misses := m.misses.Load()

	var hitRate, missRate float64
	if total := hits + misses; total > 0 {
		hitRate = float64(hits) / float64(total)
		missRate = float64(misses) / float64(total)
	}
	return Snapshot{
		Hits:      hits,
		Misses:    misses,
		Sets:      m.sets.Load(),
		Deletes:   m.deletes.Load(),
		Evictions: m.evictions.Load(),
		HitRate:   hitRate,
		MissRate:  missRate,
	}
}
