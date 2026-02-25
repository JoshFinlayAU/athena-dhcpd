package dnsproxy

import (
	"sync"
	"time"

	"github.com/miekg/dns"
)

// cacheEntry holds a cached DNS response.
type cacheEntry struct {
	msg       *dns.Msg
	expiresAt time.Time
}

// Cache is a simple TTL-based DNS response cache.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
}

// NewCache creates a cache with the given max entry count.
func NewCache(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Cache{
		entries: make(map[string]*cacheEntry, maxSize),
		maxSize: maxSize,
	}
}

func cacheKey(name string, qtype, qclass uint16) string {
	return name + "|" + dns.TypeToString[qtype] + "|" + dns.ClassToString[qclass]
}

// Get retrieves a cached response. Returns nil if not found or expired.
func (c *Cache) Get(name string, qtype, qclass uint16) *dns.Msg {
	c.mu.RLock()
	entry, ok := c.entries[cacheKey(name, qtype, qclass)]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.msg.Copy()
}

// Set stores a DNS response in the cache. TTL is derived from the answer section
// or the provided default if no answers have a TTL.
func (c *Cache) Set(msg *dns.Msg, defaultTTL time.Duration) {
	if msg == nil || len(msg.Question) == 0 {
		return
	}

	q := msg.Question[0]

	// Determine TTL from the response
	ttl := defaultTTL
	if len(msg.Answer) > 0 {
		minTTL := msg.Answer[0].Header().Ttl
		for _, rr := range msg.Answer {
			if rr.Header().Ttl < minTTL {
				minTTL = rr.Header().Ttl
			}
		}
		if minTTL > 0 {
			ttl = time.Duration(minTTL) * time.Second
		}
	}

	// Don't cache errors for long
	if msg.Rcode != dns.RcodeSuccess {
		if ttl > 30*time.Second {
			ttl = 30 * time.Second
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity (simple: just clear half the cache)
	if len(c.entries) >= c.maxSize {
		c.evictLocked()
	}

	c.entries[cacheKey(q.Name, q.Qtype, q.Qclass)] = &cacheEntry{
		msg:       msg.Copy(),
		expiresAt: time.Now().Add(ttl),
	}
}

// evictLocked removes expired entries, or half the cache if needed. Must hold mu.
func (c *Cache) evictLocked() {
	now := time.Now()
	removed := 0

	// First pass: remove expired
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
			removed++
		}
	}

	// If we didn't free enough, nuke half
	if len(c.entries) >= c.maxSize {
		target := c.maxSize / 2
		for k := range c.entries {
			delete(c.entries, k)
			if len(c.entries) <= target {
				break
			}
		}
	}
}

// Size returns the current number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Flush clears the entire cache.
func (c *Cache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry, c.maxSize)
}
