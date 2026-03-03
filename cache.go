package gokut

import (
	"sync"
	"time"
)

// NoExpiration is a sentinel TTL meaning the item lives until explicitly
// deleted or the cache is flushed.
const NoExpiration time.Duration = 0

// Cache is a generic, thread-safe in-memory key-value store with:
//
//   - Per-item TTL and an O(k) background janitor (sorted expiry linked list)
//   - Configurable eviction policies: NoEviction, FIFO, LRU, LFU
//   - Optional memory cap (bytes) or item count cap
//   - Lock-free atomic metrics (hits, misses, evictions, …)
//
// Construct with NewCache and functional options:
//
//	cache := NewCache(
//	    WithMaxItems[string, int](1000),
//	    WithEvictionPolicy[string, int](LRU),
//	    WithCleanupInterval[string, int](time.Minute),
//	)
type Cache[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]*Item[K, V]

	// ── Expiry linked list ─────────────────────────────────────────────────
	// Sorted ascending by expiresAt.
	// head = soonest to expire, tail = latest to expire.
	// Janitor walks from head and stops at the first non-expired node → O(k).
	head *expNode[K]
	tail *expNode[K]

	// ── Eviction ───────────────────────────────────────────────────────────
	policy   EvictionPolicy
	ev       evictionState[K, V]
	maxItems int // 0 = no limit

	// ── Lifecycle ──────────────────────────────────────────────────────────
	onEviction      func(K, V)
	cleanupInterval time.Duration
	stopCh          chan struct{}

	// ── Metrics (atomic — no mutex needed) ────────────────────────────────
	metrics Metrics
}

// NewCache creates a new cache and applies the supplied options.
// Call Stop() when the cache is no longer needed to release the janitor
// goroutine (if one was started via WithCleanupInterval).
func NewCache[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		items:  make(map[K]*Item[K, V]),
		stopCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.initEviction()
	if c.cleanupInterval > 0 {
		go c.runJanitor()
	}
	return c
}

// initEviction wires the policy-specific callbacks.
func (c *Cache[K, V]) initEviction() {
	switch c.policy {
	case LRU:
		c.ev = newLRUState[K, V]()
	case FIFO:
		c.ev = newFIFOState[K, V]()
	default:
		c.ev = noEvictionState[K, V]()
	}
}

// ── Public API ────────────────────────────────────────────────────────────────

// Set stores key→value with an optional TTL.
// Use ttl=NoExpiration (0) for items that should live until explicitly deleted.
//
// When the cache has reached its MaxItems limit and the policy is not
// NoEviction, Set evicts the chosen victim first, then inserts the new item.
//
// For NoEviction: when the cache is full, the new item is silently discarded.
func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the key already exists, cleanly remove it first (update semantics).
	// We do NOT fire the onEviction callback for overwrites.
	if existing, ok := c.items[key]; ok {
		c.ev.onRemove(key, existing)
		if existing.node != nil {
			c.removeNode(existing.node)
		}
		delete(c.items, key)
	}

	// For NoEviction: if adding this item would exceed the limit, drop it.
	if c.policy == NoEviction && c.maxItems > 0 && len(c.items) >= c.maxItems {
		return
	}

	// Build and insert the new item.
	item := &Item[K, V]{value: value}
	if expiresAt > 0 {
		node := &expNode[K]{key: key, expiresAt: expiresAt}
		item.node = node
		c.insertNode(node)
	}
	c.items[key] = item
	c.metrics.sets.Add(1)
	c.ev.onInsert(key, item)

	// Evict until within limits (no-op for NoEviction).
	for c.shouldEvict() {
		if !c.evictOne() {
			break
		}
	}
}

// Get retrieves an item, performing passive expiration if needed.
// Callers never observe stale data even if the janitor hasn't swept yet.
//
// For LRU and LFU, a successful Get also updates the item's position /
// frequency — which requires briefly promoting to the write lock.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		c.metrics.misses.Add(1)
		var zero V
		return zero, false
	}

	// Passive expiration check.
	if item.node != nil && time.Now().UnixNano() >= item.node.expiresAt {
		c.coreDelete(key)
		c.metrics.misses.Add(1)
		c.metrics.evictions.Add(1)
		var zero V
		return zero, false
	}

	// Live item — notify the policy (LRU promotes to MRU, LFU increments freq).
	c.ev.onAccess(key, item)
	c.metrics.hits.Add(1)
	return item.value, true
}

// Delete removes a key from the cache immediately.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.coreDelete(key) {
		c.metrics.deletes.Add(1)
	}
}

// Flush removes every item from the cache and resets memory tracking.
func (c *Cache[K, V]) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, item := range c.items {
		c.ev.onRemove(k, item)
		if c.onEviction != nil {
			c.onEviction(k, item.value)
		}
	}
	c.items = make(map[K]*Item[K, V])
	c.head = nil
	c.tail = nil
}

// Len returns the current number of items in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns a point-in-time snapshot of the cache metrics.
func (c *Cache[K, V]) Stats() Snapshot {
	return c.metrics.snapshot()
}

// Stop shuts down the background janitor goroutine.
// Safe and idempotent — may be called multiple times.
func (c *Cache[K, V]) Stop() {
	select {
	case <-c.stopCh: // already closed
	default:
		close(c.stopCh)
	}
}

// ── Internal helpers (all require the write lock to be held) ─────────────────

// shouldEvict reports whether the cache has exceeded its item count limit.
func (c *Cache[K, V]) shouldEvict() bool {
	return c.maxItems > 0 && len(c.items) > c.maxItems
}

// evictOne asks the policy for the next victim and removes it.
// Returns true if an item was evicted.
func (c *Cache[K, V]) evictOne() bool {
	key, ok := c.ev.evict()
	if !ok {
		return false
	}
	if c.coreDelete(key) {
		c.metrics.evictions.Add(1)
		return true
	}
	return false
}

// coreDelete is the single removal code-path.
// Returns true if the key existed and was removed.
func (c *Cache[K, V]) coreDelete(key K) bool {
	item, ok := c.items[key]
	if !ok {
		return false
	}
	if item.node != nil {
		c.removeNode(item.node)
	}
	c.ev.onRemove(key, item)
	delete(c.items, key)
	if c.onEviction != nil {
		c.onEviction(key, item.value)
	}
	return true
}

// deleteExpired is called by the janitor.
// It walks the expiry list from the head (soonest expiry) and removes every
// node whose deadline has passed — stopping the moment it finds a live node.
// Cost: O(k) where k = number of expired items, not O(n) total.
func (c *Cache[K, V]) deleteExpired() {
	now := time.Now().UnixNano()
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.head != nil && c.head.expiresAt <= now {
		key := c.head.key // capture before coreDelete advances c.head
		c.coreDelete(key)
		c.metrics.evictions.Add(1)
	}
}

// ── Expiry linked-list operations (write lock required) ──────────────────────

// insertNode inserts n into the expiry list, preserving ascending order on
// expiresAt.
//
// O(1) fast path: new node expires latest → append to tail.
// O(k) slow path: scan from tail backwards to find the insertion point.
func (c *Cache[K, V]) insertNode(n *expNode[K]) {
	if c.tail == nil || n.expiresAt >= c.tail.expiresAt {
		c.appendTail(n)
		return
	}
	if n.expiresAt <= c.head.expiresAt {
		c.prependHead(n)
		return
	}
	cur := c.tail
	for cur != nil && cur.expiresAt > n.expiresAt {
		cur = cur.prev
	}
	c.insertAfter(n, cur)
}

func (c *Cache[K, V]) appendTail(n *expNode[K]) {
	n.prev = c.tail
	n.next = nil
	if c.tail != nil {
		c.tail.next = n
	} else {
		c.head = n
	}
	c.tail = n
}

func (c *Cache[K, V]) prependHead(n *expNode[K]) {
	n.next = c.head
	n.prev = nil
	if c.head != nil {
		c.head.prev = n
	} else {
		c.tail = n
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

// removeNode splices n out of the expiry list in O(1).
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
