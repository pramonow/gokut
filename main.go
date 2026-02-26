package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== In-Memory Cache Demo ===")
	fmt.Println()

	// Start a cache with a janitor that sweeps every 500 ms.
	cache := NewCache[string, int](500 * time.Millisecond)
	defer cache.Stop()

	// Log every eviction so we can see the linked-list janitor at work.
	cache.SetOnEviction(func(key string, value int) {
		fmt.Printf("  [eviction] key=%q  value=%d\n", key, value)
	})

	// ── Set items with different TTLs ─────────────────────────────────────────
	fmt.Println("Setting items...")
	cache.Set("alice", 25, 2*time.Second)   // expires in 2 s
	cache.Set("bob", 30, 4*time.Second)     // expires in 4 s
	cache.Set("charlie", 35, 6*time.Second) // expires in 6 s
	cache.Set("diana", 40, NoExpiration)    // lives forever

	fmt.Printf("Items in cache: %d\n\n", cache.Len())

	// ── Immediate reads ───────────────────────────────────────────────────────
	fmt.Println("Immediate reads:")
	for _, name := range []string{"alice", "bob", "charlie", "diana"} {
		if v, ok := cache.Get(name); ok {
			fmt.Printf("  %-10s → %d\n", name, v)
		}
	}
	fmt.Println()

	// ── Overwrite alice with a fresh TTL ─────────────────────────────────────
	fmt.Println("Overwriting 'alice' with new value+TTL (3 s)...")
	cache.Set("alice", 99, 3*time.Second) // old expiry node removed from list
	if v, ok := cache.Get("alice"); ok {
		fmt.Printf("  alice → %d (refreshed)\n\n", v)
	}

	// ── Wait for alice & bob to expire ───────────────────────────────────────
	fmt.Println("Sleeping 3.5 s — alice should expire, bob should still be alive...")
	time.Sleep(3500 * time.Millisecond)
	fmt.Println()

	for _, name := range []string{"alice", "bob", "charlie", "diana"} {
		if v, ok := cache.Get(name); ok {
			fmt.Printf("  %-10s → %d\n", name, v)
		} else {
			fmt.Printf("  %-10s → (expired / not found)\n", name)
		}
	}
	fmt.Printf("\nItems remaining: %d\n\n", cache.Len())

	// ── Wait for bob to expire too ────────────────────────────────────────────
	fmt.Println("Sleeping another 1 s — bob expires...")
	time.Sleep(1000 * time.Millisecond)
	fmt.Println()

	for _, name := range []string{"bob", "charlie", "diana"} {
		if v, ok := cache.Get(name); ok {
			fmt.Printf("  %-10s → %d\n", name, v)
		} else {
			fmt.Printf("  %-10s → (expired)\n", name)
		}
	}
	fmt.Printf("\nItems remaining: %d\n\n", cache.Len())

	// ── Flush everything ──────────────────────────────────────────────────────
	fmt.Println("Flushing all remaining items...")
	cache.Flush()
	fmt.Printf("Items after flush: %d\n", cache.Len())
}
