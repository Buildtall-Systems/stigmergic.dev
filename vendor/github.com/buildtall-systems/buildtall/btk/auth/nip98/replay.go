package nip98

import (
	"sync"
	"time"
)

// ReplayTTLFactor scales the skew window into a replay-cache TTL: an entry
// must outlive the window in which its event would still verify.
const ReplayTTLFactor = 2

// ReplayCache records event IDs so each verified auth event grants access
// exactly once. It is safe for concurrent use. Entries expire after the TTL,
// which must cover the full clock-skew window on both sides.
type ReplayCache struct {
	seen map[string]time.Time
	now  func() time.Time
	mu   sync.Mutex
	ttl  time.Duration
}

// NewReplayCache returns a cache whose entries expire after ttl. A
// non-positive ttl falls back to ReplayTTLFactor times DefaultMaxSkew.
func NewReplayCache(ttl time.Duration) *ReplayCache {
	if ttl <= 0 {
		ttl = ReplayTTLFactor * DefaultMaxSkew
	}
	return &ReplayCache{
		seen: make(map[string]time.Time),
		ttl:  ttl,
		now:  time.Now,
	}
}

// Seen reports whether id was already recorded and, if not, records it.
// Expired entries are swept on each call.
func (c *ReplayCache) Seen(id string) bool {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, expiry := range c.seen {
		if now.After(expiry) {
			delete(c.seen, k)
		}
	}

	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = now.Add(c.ttl)
	return false
}
