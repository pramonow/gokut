package main

import (
	"sync"
	"time"
)

// NoExpiration is a sentinel value to pass as ttl when you want an item to
// live forever (i.e. it is only removed by an explicit Delete or Flush).
const NoExpiration time.Duration = 0

// Cache is a generic, thread-safe in-memory key-value store with per-item TTL.
//
// Expiry optimisation — sorted doubly-linked list:
//
//	Items that carry a TTL are also tracked in a doubly-linked list that is
//	kept sorted in ascending order by expiresAt (head = soonest to expire).
//	The background janitor starts at the head and walks forward, removing
//	nodes until it finds one that has not yet expired.  Cost is O(k) where
//	k is the number of expired items, not O(n) for the whole map.
//	Passive expiration in Get() also uses this list to remove a node in O(1).
type Cache[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]*Item[K, V]

	// Expiry doubly-linked list.
	// head → expiresAt is smallest (soonest)
	// tail → expiresAt is largest (latest)
	head *expNode[K]
	tail *expNode[K]

	onEviction      func(key K, value V)
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

// NewCache creates a cache and starts a background janitor that runs every
// cleanupInterval.  Pass cleanupInterval=0 to disable the janitor entirely
// (passive expiration in Get() still works).
// Call Stop() when the cache is no longer needed to free the goroutine.
func NewCache[K comparable, V any](cleanupInterval time.Duration) *Cache[K, V] {
	c := &Cache[K, V]{
		items:           make(map[K]*Item[K, V]),
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}
	if cleanupInterval > 0 {
		go c.runJanitor()
	}
	return c
}

// SetOnEviction registers a callback that is invoked whenever an item is
// removed from the cache for any reason (TTL expiry, Delete, or Flush).
func (c *Cache[K, V]) SetOnEviction(fn func(key K, value V)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEviction = fn
}

// ── Public API ────────────────────────────────────────────────────────────────

// Set stores key→value with an optional TTL.
// Use ttl=NoExpiration (0) for items that should live until explicitly deleted.
//
// If the key already exists its old expiry node is cleanly removed from the
// linked list before inserting the new one, so there are no stale nodes.
func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove old expiry node if this key is being overwritten.
	if existing, ok := c.items[key]; ok && existing.node != nil {
		c.removeNode(existing.node)
	}

	item := &Item[K, V]{value: value}
	if expiresAt > 0 {
		node := &expNode[K]{key: key, expiresAt: expiresAt}
		item.node = node
		c.insertNode(node) // keeps list sorted ascending
	}
	c.items[key] = item
}

// Get retrieves an item.  It performs passive expiration: if the item is found
// but its TTL has already elapsed, it is deleted on the spot and a cache miss
// is returned — so callers never observe stale data even if the janitor has not
// run yet.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	// We need a full write lock because a passive expiry modifies the map and
	// the linked list.  For the hot path (non-expired items) we acquire the
	// read lock first, and only promote to a write lock when needed.
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		var zero V
		return zero, false
	}

	// Happy path — item exists and has not expired (or has no TTL).
	if item.node == nil || time.Now().UnixNano() < item.node.expiresAt {
		return item.value, true
	}

	// Expired — promote to write lock and clean up.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check after acquiring the write lock (another goroutine may have
	// already handled this item between our RUnlock and Lock).
	item, ok = c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	if item.node != nil && time.Now().UnixNano() >= item.node.expiresAt {
		c.removeNode(item.node)
		delete(c.items, key)
		if c.onEviction != nil {
			c.onEviction(key, item.value)
		}
		var zero V
		return zero, false
	}

	// Wasn't expired after all (race was won by another goroutine that
	// refreshed the item).  Return the current value.
	return item.value, true
}

// Delete removes a key from the cache immediately.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delete(key)
}

// Flush removes every item from the cache.
func (c *Cache[K, V]) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.onEviction != nil {
		for k, item := range c.items {
			c.onEviction(k, item.value)
		}
	}
	c.items = make(map[K]*Item[K, V])
	c.head = nil
	c.tail = nil
}

// Len returns the current number of items in the cache (including items whose
// TTL has elapsed but have not been cleaned up yet by the janitor).
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stop shuts down the background janitor goroutine.
// It is safe (and a no-op) to call Stop on a cache with no janitor.
func (c *Cache[K, V]) Stop() {
	select {
	case <-c.stopCh: // already closed
	default:
		close(c.stopCh)
	}
}

// ── Internal helpers (all require the write lock to be held) ─────────────────

// delete is the internal delete — caller must hold the write lock.
func (c *Cache[K, V]) delete(key K) {
	item, ok := c.items[key]
	if !ok {
		return
	}
	if item.node != nil {
		c.removeNode(item.node)
	}
	delete(c.items, key)
	if c.onEviction != nil {
		c.onEviction(key, item.value)
	}
}

// deleteExpired is called by the janitor.
//
// Because the linked list is sorted ascending by expiresAt, we only need to
// walk from the head and stop as soon as we find a node that has not expired.
// This makes the janitor sweep O(k) instead of O(n).
func (c *Cache[K, V]) deleteExpired() {
	now := time.Now().UnixNano()

	c.mu.Lock()
	defer c.mu.Unlock()

	for c.head != nil && c.head.expiresAt <= now {
		key := c.head.key
		item := c.items[key]
		c.removeNode(c.head) // advances c.head
		delete(c.items, key)
		if c.onEviction != nil && item != nil {
			c.onEviction(key, item.value)
		}
	}
}

// ── Linked-list operations (all require the write lock to be held) ────────────

// insertNode inserts n into the expiry list, preserving ascending order on
// expiresAt.
//
// Fast path (O(1)): when n.expiresAt >= tail.expiresAt we simply append to
// the tail.  This covers the common real-world case where items are added with
// the same (or increasing) TTLs.
//
// Slow path (O(k)): for out-of-order expirations we scan from the tail
// backwards, since the new node is more likely to be near the end than the
// beginning.
func (c *Cache[K, V]) insertNode(n *expNode[K]) {
	// List is empty, or new node expires latest — append to tail.
	if c.tail == nil || n.expiresAt >= c.tail.expiresAt {
		c.appendTail(n)
		return
	}

	// New node expires earliest of all — prepend to head.
	if n.expiresAt <= c.head.expiresAt {
		c.prependHead(n)
		return
	}

	// Scan from tail backwards until we find the right insertion point.
	cur := c.tail
	for cur != nil && cur.expiresAt > n.expiresAt {
		cur = cur.prev
	}
	// cur.expiresAt <= n.expiresAt < cur.next.expiresAt  →  insert after cur.
	c.insertAfter(n, cur)
}

func (c *Cache[K, V]) appendTail(n *expNode[K]) {
	n.prev = c.tail
	n.next = nil
	if c.tail != nil {
		c.tail.next = n
	} else {
		c.head = n // list was empty
	}
	c.tail = n
}

func (c *Cache[K, V]) prependHead(n *expNode[K]) {
	n.next = c.head
	n.prev = nil
	if c.head != nil {
		c.head.prev = n
	} else {
		c.tail = n // list was empty
	}
	c.head = n
}

func (c *Cache[K, V]) insertAfter(n, at *expNode[K]) {
	n.prev = at
	n.next = at.next
	if at.next != nil {
		at.next.prev = n
	} else {
		c.tail = n
	}
	at.next = n
}

// removeNode splices n out of the list in O(1).
func (c *Cache[K, V]) removeNode(n *expNode[K]) {
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		c.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		c.tail = n.prev
	}
	n.prev = nil
	n.next = nil
}
