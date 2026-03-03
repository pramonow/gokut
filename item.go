package gokut

// expNode is a node in the expiry-sorted doubly-linked list (sorted by
// expiresAt ascending).  It carries only what the janitor needs so the list
// stays as lean as possible.
type expNode[K comparable] struct {
	key       K
	expiresAt int64 // Unix nanoseconds; 0 = never expires
	prev      *expNode[K]
	next      *expNode[K]
}

// Item is the value stored in the hash map for each cache key.
// It holds the actual value, an optional pointer into the expiry linked list,
// an optional pointer into the eviction policy list, and a tracked byte size.
type Item[K comparable, V any] struct {
	value      V
	node       *expNode[K]    // expiry list node; nil when item has no TTL
	policyNode *policyNode[K] // eviction policy list node; nil for NoEviction
}
