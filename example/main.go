package main

import (
	"fmt"
	"time"

	"github.com/pramonow/gokut"
)

func main() {
	demoLRU()
	demoFIFO()
	demoTTLWithEviction()
}

func demoLRU() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: LRU  (max 3 items)")
	fmt.Println("═══════════════════════════════════════")

	cache := gokut.NewCache(
		gokut.WithMaxItems[string, int](3),
		gokut.WithEvictionPolicy[string, int](gokut.LRU),
		gokut.WithOnEviction[string, int](func(k string, v int) {
			fmt.Printf("  [evicted] %-10s = %d\n", k, v)
		}),
	)
	defer cache.Stop()

	cache.Set("alice", 1, gokut.NoExpiration)
	cache.Set("bob", 2, gokut.NoExpiration)
	cache.Set("charlie", 3, gokut.NoExpiration)

	fmt.Println("Access 'alice' and 'charlie' → bob becomes LRU")
	cache.Get("alice")
	cache.Get("charlie")

	fmt.Println("Insert 'diana=4' → bob should be evicted (LRU)")
	cache.Set("diana", 4, gokut.NoExpiration)

	printState(cache, []string{"alice", "bob", "charlie", "diana"})
	fmt.Println("Stats:", cache.Stats())
	fmt.Println()
}

func demoFIFO() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: FIFO  (max 3 items)")
	fmt.Println("═══════════════════════════════════════")

	cache := gokut.NewCache(
		gokut.WithMaxItems[string, int](3),
		gokut.WithEvictionPolicy[string, int](gokut.FIFO),
		gokut.WithOnEviction[string, int](func(k string, v int) {
			fmt.Printf("  [evicted] %-10s = %d\n", k, v)
		}),
	)
	defer cache.Stop()

	cache.Set("alice", 1, gokut.NoExpiration)
	cache.Set("bob", 2, gokut.NoExpiration)
	cache.Set("charlie", 3, gokut.NoExpiration)

	fmt.Println("Access all items (FIFO ignores access order)...")
	cache.Get("charlie")
	cache.Get("alice")
	cache.Get("bob")

	fmt.Println("Insert 'diana=4' → alice should be evicted (first inserted)")
	cache.Set("diana", 4, gokut.NoExpiration)

	printState(cache, []string{"alice", "bob", "charlie", "diana"})
	fmt.Println("Stats:", cache.Stats())
	fmt.Println()
}

func demoTTLWithEviction() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  DEMO: TTL + LRU together")
	fmt.Println("═══════════════════════════════════════")

	cache := gokut.NewCache(
		gokut.WithMaxItems[string, string](2),
		gokut.WithEvictionPolicy[string, string](gokut.LRU),
		gokut.WithCleanupInterval[string, string](200*time.Millisecond),
		gokut.WithOnEviction[string, string](func(k, v string) {
			fmt.Printf("  [evicted] %s\n", k)
		}),
	)
	defer cache.Stop()

	cache.Set("short", "expires soon", 300*time.Millisecond)
	cache.Set("long", "lives long", 5*time.Second)

	fmt.Println("Cache state (immediate):")
	printStateStr(cache, []string{"short", "long"})

	fmt.Println("Sleeping 500 ms — 'short' expires, 'new' fits in the gap")
	time.Sleep(500 * time.Millisecond)
	cache.Set("new", "just arrived", gokut.NoExpiration)
	printStateStr(cache, []string{"short", "long", "new"})

	fmt.Println("Stats:", cache.Stats())
}

func printState(c *gokut.Cache[string, int], keys []string) {
	fmt.Println("Cache state:")
	for _, k := range keys {
		if v, ok := c.Get(k); ok {
			fmt.Printf("  %-10s → %d\n", k, v)
		} else {
			fmt.Printf("  %-10s → (not found)\n", k)
		}
	}
}

func printStateStr(c *gokut.Cache[string, string], keys []string) {
	fmt.Println("Cache state:")
	for _, k := range keys {
		if v, ok := c.Get(k); ok {
			fmt.Printf("  %-8s → %q\n", k, v)
		} else {
			fmt.Printf("  %-8s → (not found)\n", k)
		}
	}
}
