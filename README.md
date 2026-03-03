# gokut 🇮🇩

> High performance, thread-safe, in-memory cache for Go, why not re-invent the wheel when i can?

A generic, thread-safe, high-performance in-memory cache for Go — with optional TTL, configurable eviction policies, and an O(k) janitor that only does as much work as necessary.

---

## Why gokut?

When your stock gets stuck and you just have to hold, the usual terminology in my language (Indonesian) is *nyangkut*, that's where the inspiration from.

So hence go (nyang)kut.

gokut turns that pain into a feature: your data **stays put** in RAM until *you* decide to evict it — by policy, by TTL, or by force.

```
Your portfolio:  📉 nyangkut forever
Your cache:      ✅ nyangkut with LRU, TTL, and a janitor
```

---

## Features

- 🔒 **Thread-safe** — `sync.RWMutex`, safe for concurrent reads
- 🧬 **Generic** — `Cache[K comparable, V any]`, no `interface{}` boxing
- ⏱️ **Optional TTL** — per-item expiry, or `NoExpiration` to hold forever
- 🧹 **O(k) Janitor** — sorted expiry linked list; cleanup cost = number of *expired* items, not total items
- 📋 **Eviction policies** — `LRU`, `FIFO`, `NoEviction`
- 📊 **Atomic metrics** — hit rate, miss rate, eviction count with zero mutex overhead
- 🔔 **Eviction callback** — hook into every removal (logging, resource cleanup, etc.)

---

## Installation

```bash
go get github.com/pramonow/gokut
```

---

## Quick Start

```go
package main

import (
    "fmt"
    "time"
    "github.com/pramonow/gokut"
)

func main() {
    cache := gokut.NewCache(
        gokut.WithMaxItems[string, int](1000),
        gokut.WithEvictionPolicy[string, int](gokut.LRU),
        gokut.WithCleanupInterval[string, int](time.Minute),
    )
    defer cache.Stop()

    // Set with TTL
    cache.Set("session:abc", 42, 30*time.Minute)

    // Set forever (NoExpiration)
    cache.Set("config:timeout", 5000, gokut.NoExpiration)

    // Get (passive expiry check included)
    if val, ok := cache.Get("session:abc"); ok {
        fmt.Println("got:", val)
    }

    // Delete
    cache.Delete("session:abc")

    // Metrics
    fmt.Println(cache.Stats())
}
```

---

## Options

| Option | Description |
|---|---|
| `WithMaxItems[K,V](n int)` | Evict when item count exceeds `n` |
| `WithEvictionPolicy[K,V](p)` | `LRU` \| `FIFO` \| `NoEviction` (default) |
| `WithCleanupInterval[K,V](d)` | How often the background janitor sweeps expired items |
| `WithOnEviction[K,V](fn)` | Callback fired on every removal (TTL, policy, Delete, Flush) |

---

## Eviction Policies

### LRU — Least Recently Used
The item that was accessed furthest in the past gets evicted first.  
Both `Get()` and `Set()` count as accesses.

```go
cache := gokut.NewCache(
    gokut.WithMaxItems[string, string](3),
    gokut.WithEvictionPolicy[string, string](gokut.LRU),
)
cache.Set("a", "1", gokut.NoExpiration)
cache.Set("b", "2", gokut.NoExpiration)
cache.Set("c", "3", gokut.NoExpiration)
cache.Get("a") // promotes "a" to MRU
cache.Get("c") // promotes "c" to MRU — "b" is now LRU
cache.Set("d", "4", gokut.NoExpiration) // evicts "b"
```

### FIFO — First In, First Out
The item inserted earliest is evicted first, regardless of how often it was accessed.

```go
cache := gokut.NewCache(
    gokut.WithMaxItems[string, string](3),
    gokut.WithEvictionPolicy[string, string](gokut.FIFO),
)
```

### NoEviction (default)
When the cache is full, new items are **silently discarded**.  
Useful when you want a fixed-size warm cache and can tolerate misses.

---

## TTL

```go
// Expires in 5 minutes
cache.Set("key", value, 5*time.Minute)

// Lives until explicitly deleted or cache is flushed
cache.Set("key", value, gokut.NoExpiration)
```

**Passive expiration**: `Get()` checks the TTL before returning. Expired items are deleted on the spot — callers never see stale data even if the janitor hasn't run yet.

**Active expiration (Janitor)**: A background goroutine wakes up at `cleanupInterval` and sweeps expired items from the head of a sorted linked list. Cost is O(k) where k = number of expired items.

---

## Metrics

```go
stats := cache.Stats()
fmt.Printf("HitRate: %.1f%%  Misses: %d  Evictions: %d\n",
    stats.HitRate*100, stats.Misses, stats.Evictions)
```

| Field | Description |
|---|---|
| `Hits` | Successful `Get()` calls |
| `Misses` | `Get()` calls that found nothing (absent or expired) |
| `HitRate` | `Hits / (Hits + Misses)` |
| `MissRate` | `Misses / (Hits + Misses)` |
| `Sets` | Total `Set()` calls |
| `Deletes` | Explicit `Delete()` calls |
| `Evictions` | Items removed by TTL or eviction policy |

All counters use `sync/atomic` — no mutex contention on the hot path.

---

## API Reference

```go
func NewCache[K comparable, V any](opts ...Option[K, V]) *Cache[K, V]

func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration)
func (c *Cache[K, V]) Get(key K) (V, bool)
func (c *Cache[K, V]) Delete(key K)
func (c *Cache[K, V]) Flush()
func (c *Cache[K, V]) Len() int
func (c *Cache[K, V]) Stats() Snapshot
func (c *Cache[K, V]) Stop()
```

---

## How the janitor works

```
Expiry list (sorted ascending by expiresAt):

  head → [a, t=1s] → [b, t=3s] → [c, t=10s] → nil
                                                 ↑
                                                tail

Janitor wakes up, now = t=2s:
  ✓ removes [a] (expired)
  ✗ stops at [b] (not yet expired)

Cost: O(1) — only 1 item checked, not all 3.
```

---

## Run the example

```bash
git clone https://github.com/pramonow/gokut
cd gokut/example
go run .
```

---

## License

MIT

---

*"Nyangkut di RAM, bukan di saham."* 🇮🇩
