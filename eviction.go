package gokut

// EvictionPolicy determines which item is removed when the cache is full.
type EvictionPolicy int

const (
	// NoEviction silently discards new items when the cache is at its limit.
	NoEviction EvictionPolicy = iota
	// FIFO evicts the item that was inserted earliest.
	FIFO
	// LRU evicts the item that was accessed (Get or Set) least recently.
	LRU
)

// ── policyNode ────────────────────────────────────────────────────────────────

// policyNode is a node in the eviction policy's doubly-linked list.
type policyNode[K comparable] struct {
	key  K
	prev *policyNode[K]
	next *policyNode[K]
}

// ── evictionState ─────────────────────────────────────────────────────────────

// evictionState bundles the four callbacks that implement a specific eviction
// policy.  All callbacks are invoked with the cache write-lock already held.
type evictionState[K comparable, V any] struct {
	// onInsert is called right after a new item is stored in the cache.
	onInsert func(key K, item *Item[K, V])
	// onAccess is called on every successful Get() hit.
	onAccess func(key K, item *Item[K, V])
	// onRemove is called just before an item is deleted for any reason.
	onRemove func(key K, item *Item[K, V])
	// evict returns the key of the next candidate for eviction.
	evict func() (K, bool)
}

// ── Shared doubly-linked list (used by both FIFO and LRU) ────────────────────

// policyList is a doubly-linked list where:
//   - head is the EVICTION end  (LRU = least recently used, FIFO = oldest insert)
//   - tail is the KEEP end      (MRU = most recently used, FIFO = newest insert)
type policyList[K comparable] struct {
	head *policyNode[K]
	tail *policyNode[K]
}

func (l *policyList[K]) pushTail(n *policyNode[K]) {
	n.prev = l.tail
	n.next = nil
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
}

func (l *policyList[K]) remove(n *policyNode[K]) {
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
	n.prev = nil
	n.next = nil
}

func (l *policyList[K]) moveToTail(n *policyNode[K]) {
	if l.tail == n {
		return // already at MRU end
	}
	l.remove(n)
	l.pushTail(n)
}

// ── FIFO ──────────────────────────────────────────────────────────────────────

// newFIFOState returns an evictionState that evicts the oldest-inserted item.
// Access order does not affect eviction priority.
func newFIFOState[K comparable, V any]() evictionState[K, V] {
	list := &policyList[K]{}
	return evictionState[K, V]{
		onInsert: func(key K, item *Item[K, V]) {
			n := &policyNode[K]{key: key}
			item.policyNode = n
			list.pushTail(n)
		},
		onAccess: func(key K, item *Item[K, V]) {
			// FIFO does not reorder on access.
		},
		onRemove: func(key K, item *Item[K, V]) {
			if item.policyNode != nil {
				list.remove(item.policyNode)
				item.policyNode = nil
			}
		},
		evict: func() (K, bool) {
			if list.head == nil {
				var zero K
				return zero, false
			}
			return list.head.key, true
		},
	}
}

// ── LRU ───────────────────────────────────────────────────────────────────────

// newLRUState returns an evictionState that evicts the least recently used item.
// Both Get() hits and Set() calls promote an item to the MRU (tail) position.
func newLRUState[K comparable, V any]() evictionState[K, V] {
	list := &policyList[K]{}
	return evictionState[K, V]{
		onInsert: func(key K, item *Item[K, V]) {
			n := &policyNode[K]{key: key}
			item.policyNode = n
			list.pushTail(n) // new items start as MRU
		},
		onAccess: func(key K, item *Item[K, V]) {
			if item.policyNode != nil {
				list.moveToTail(item.policyNode) // promote to MRU
			}
		},
		onRemove: func(key K, item *Item[K, V]) {
			if item.policyNode != nil {
				list.remove(item.policyNode)
				item.policyNode = nil
			}
		},
		evict: func() (K, bool) {
			if list.head == nil {
				var zero K
				return zero, false
			}
			return list.head.key, true // head = LRU
		},
	}
}

// ── NoEviction ────────────────────────────────────────────────────────────────

// noEvictionState returns a no-op state.  When the cache is full, new items
// are silently discarded — nothing is ever evicted.
func noEvictionState[K comparable, V any]() evictionState[K, V] {
	return evictionState[K, V]{
		onInsert: func(K, *Item[K, V]) {},
		onAccess: func(K, *Item[K, V]) {},
		onRemove: func(K, *Item[K, V]) {},
		evict: func() (K, bool) {
			var zero K
			return zero, false
		},
	}
}
