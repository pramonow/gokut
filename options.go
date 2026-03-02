package main

import "time"

// Option is a functional option that configures a Cache at construction time.
// Using functional options keeps NewCache's signature clean and lets callers
// mix and match only the settings they care about.
type Option[K comparable, V any] func(*Cache[K, V])

// WithCleanupInterval sets how often the background janitor scans for and
// removes TTL-expired items.  Pass 0 to disable the janitor (passive
// expiration in Get() still works regardless).
func WithCleanupInterval[K comparable, V any](d time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.cleanupInterval = d
	}
}

// WithMaxItems caps the number of items the cache may hold.
// When the cap is exceeded, the configured EvictionPolicy decides what
// gets removed (default: NoEviction — new items are silently discarded).
func WithMaxItems[K comparable, V any](n int) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.maxItems = n
	}
}

// when the cache exceeds its configured limits.
// Default is NoEviction (new items are silently dropped when full).
func WithEvictionPolicy[K comparable, V any](p EvictionPolicy) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.policy = p
	}
}

// WithOnEviction registers a callback invoked whenever an item is removed
// for any reason: TTL expiry, eviction policy, explicit Delete, or Flush.
func WithOnEviction[K comparable, V any](fn func(K, V)) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.onEviction = fn
	}
}
