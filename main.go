package main

import (
	"fmt"
	"time"
)

func main() {
	demoLRU()
	demoFIFO()
	demoTTLWithEviction()
}

// ── 1. LRU ────────────────────────────────────────────────────────────────────

func demoLRU() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: LRU  (max 3 items)")
	fmt.Println("═══════════════════════════════════════")

	cache := NewCache(
		WithMaxItems[string, int](3),
		WithEvictionPolicy[string, int](LRU),
		WithOnEviction[string, int](func(k string, v int) {
			fmt.Printf("  [evicted] %-10s = %d\n", k, v)
		}),
	)
	defer cache.Stop()

	fmt.Println("Insert: alice=1, bob=2, charlie=3")
	cache.Set("alice", 1, NoExpiration)
	cache.Set("bob", 2, NoExpiration)
	cache.Set("charlie", 3, NoExpiration)

	fmt.Println("Access 'alice' and 'charlie' → bob becomes LRU")
	cache.Get("alice")
	cache.Get("charlie")

	fmt.Println("Insert 'diana=4' → bob should be evicted (LRU)")
	cache.Set("diana", 4, NoExpiration)

	printState(cache, []string{"alice", "bob", "charlie", "diana"})
	fmt.Println("Stats:", cache.Stats())
	fmt.Println()
}

// ── 2. FIFO ───────────────────────────────────────────────────────────────────

func demoFIFO() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: FIFO  (max 3 items)")
	fmt.Println("═══════════════════════════════════════")

	cache := NewCache(
		WithMaxItems[string, int](3),
		WithEvictionPolicy[string, int](FIFO),
		WithOnEviction[string, int](func(k string, v int) {
			fmt.Printf("  [evicted] %-10s = %d\n", k, v)
		}),
	)
	defer cache.Stop()

	fmt.Println("Insert: alice=1, bob=2, charlie=3")
	cache.Set("alice", 1, NoExpiration)
	cache.Set("bob", 2, NoExpiration)
	cache.Set("charlie", 3, NoExpiration)

	fmt.Println("Access all items (FIFO ignores access order)...")
	cache.Get("charlie")
	cache.Get("alice")
	cache.Get("bob")

	fmt.Println("Insert 'diana=4' → alice should be evicted (first inserted)")
	cache.Set("diana", 4, NoExpiration)

	printState(cache, []string{"alice", "bob", "charlie", "diana"})
	fmt.Println("Stats:", cache.Stats())
	fmt.Println()
}

// ── 3. TTL + LRU together ─────────────────────────────────────────────────────

func demoTTLWithEviction() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: TTL + LRU together")
	fmt.Println("═══════════════════════════════════════")

	cache := NewCache(
		WithMaxItems[string, string](2),
		WithEvictionPolicy[string, string](LRU),
		WithCleanupInterval[string, string](200*time.Millisecond),
		WithOnEviction[string, string](func(k, v string) {
			fmt.Printf("  [evicted] %s\n", k)
		}),
	)
	defer cache.Stop()

	cache.Set("short", "expires soon", 300*time.Millisecond)
	cache.Set("long", "lives long", 5*time.Second)

	fmt.Println("Immediately: short=✓, long=✓")
	printStateStr(cache, []string{"short", "long"})

	fmt.Println("Sleeping 500 ms — 'short' expires, 'new' fits without LRU eviction")
	time.Sleep(500 * time.Millisecond)
	cache.Set("new", "just arrived", NoExpiration)
	printStateStr(cache, []string{"short", "long", "new"})

	fmt.Println("Stats:", cache.Stats())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func printState(c *Cache[string, int], keys []string) {
	fmt.Println("Cache state:")
	for _, k := range keys {
		if v, ok := c.Get(k); ok {
			fmt.Printf("  %-10s → %d\n", k, v)
		} else {
			fmt.Printf("  %-10s → (not found)\n", k)
		}
	}
}

func printStateStr(c *Cache[string, string], keys []string) {
	fmt.Println("Cache state:")
	for _, k := range keys {
		if v, ok := c.Get(k); ok {
			fmt.Printf("  %-8s → %q\n", k, v)
		} else {
			fmt.Printf("  %-8s → (not found)\n", k)
		}
	}
}
