package main

// expNode is a single node in the expiry-ordered doubly linked list.
// It lives outside of Item so the list stays lean — it only carries what
// the janitor needs (key + expiry timestamp) and nothing else.
type expNode[K comparable] struct {
	key       K
	expiresAt int64 // Unix nanoseconds. 0 means "never expires".
	prev      *expNode[K]
	next      *expNode[K]
}

// Item is the value stored in the hash map for each cache key.
// It holds the actual cached value and an optional pointer back into the
// expiry linked list (nil when the item has no TTL).
type Item[K comparable, V any] struct {
	value V
	node  *expNode[K] // nil → this item never expires
}
