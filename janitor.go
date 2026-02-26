package main

import "time"

// runJanitor ticks at the configured cleanupInterval and calls deleteExpired
// each time.  It exits cleanly when Stop() closes the stopCh channel.
func (c *Cache[K, V]) runJanitor() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		case <-c.stopCh:
			return
		}
	}
}
