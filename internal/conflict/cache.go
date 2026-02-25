package conflict

import (
	"net"
	"sync"
	"time"
)

// ProbeCache caches recent clear probe results to avoid re-probing.
// Uses sync.Map-style concurrent-safe structure with TTL-based expiry.
type ProbeCache struct {
	entries sync.Map // IP string → *cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	clear     bool
	timestamp time.Time
}

// NewProbeCache creates a new probe result cache with the given TTL.
func NewProbeCache(ttl time.Duration) *ProbeCache {
	return &ProbeCache{
		ttl: ttl,
	}
}

// MarkClear records that an IP was probed and found clear.
func (c *ProbeCache) MarkClear(ip net.IP) {
	c.entries.Store(ip.String(), &cacheEntry{
		clear:     true,
		timestamp: time.Now(),
	})
}

// MarkConflict records that an IP was probed and found in conflict.
func (c *ProbeCache) MarkConflict(ip net.IP) {
	c.entries.Store(ip.String(), &cacheEntry{
		clear:     false,
		timestamp: time.Now(),
	})
}

// IsClear returns true if the IP was recently probed and found clear (within TTL).
func (c *ProbeCache) IsClear(ip net.IP) bool {
	v, ok := c.entries.Load(ip.String())
	if !ok {
		return false
	}
	entry := v.(*cacheEntry)
	if time.Since(entry.timestamp) > c.ttl {
		c.entries.Delete(ip.String())
		return false
	}
	return entry.clear
}

// IsConflict returns true if the IP was recently probed and found in conflict (within TTL).
func (c *ProbeCache) IsConflict(ip net.IP) bool {
	v, ok := c.entries.Load(ip.String())
	if !ok {
		return false
	}
	entry := v.(*cacheEntry)
	if time.Since(entry.timestamp) > c.ttl {
		c.entries.Delete(ip.String())
		return false
	}
	return !entry.clear
}

// Invalidate removes an IP from the cache (e.g., on DHCPDECLINE).
func (c *ProbeCache) Invalidate(ip net.IP) {
	c.entries.Delete(ip.String())
}

// Cleanup removes expired entries. Call periodically.
func (c *ProbeCache) Cleanup() {
	now := time.Now()
	c.entries.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.Sub(entry.timestamp) > c.ttl {
			c.entries.Delete(key)
		}
		return true
	})
}
