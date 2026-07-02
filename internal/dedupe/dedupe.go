// Package dedupe provides a small in-memory, TTL-based cache used to
// suppress QSOs the bridge has already forwarded. This is a client-side
// optimization only -- the logger backend also dedupes (see
// BridgeIngestController::findRecentDuplicate) so cross-instance duplicates
// (e.g. two networked N1MM PCs in a multi-op contest, each running their own
// bridge) are still caught even if this cache never sees them both.
package dedupe

import (
	"sync"
	"time"
)

type Cache struct {
	mu  sync.Mutex
	ttl time.Duration
	seen map[string]time.Time
}

// New creates a cache that remembers a key for ttl before it's eligible to
// be reported as new again.
func New(ttl time.Duration) *Cache {
	return &Cache{
		ttl:  ttl,
		seen: make(map[string]time.Time),
	}
}

// SeenRecently reports whether key was recorded within the last ttl, and
// records it now regardless (so the window slides on repeated hits).
func (c *Cache) SeenRecently(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	last, ok := c.seen[key]
	c.seen[key] = now

	if len(c.seen) > 2000 {
		c.sweepLocked(now)
	}

	return ok && now.Sub(last) < c.ttl
}

// sweepLocked drops entries older than ttl. Caller must hold the lock.
func (c *Cache) sweepLocked(now time.Time) {
	for k, t := range c.seen {
		if now.Sub(t) > c.ttl {
			delete(c.seen, k)
		}
	}
}
