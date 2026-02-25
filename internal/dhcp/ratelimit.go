package dhcp

import (
	"net"
	"sync"
	"time"
)

// RateLimiter provides token-bucket rate limiting for DHCP requests.
// Limits both global discovers/sec and per-MAC discovers/sec.
type RateLimiter struct {
	enabled           bool
	globalLimit       int
	perMACLimit       int
	globalTokens      int
	perMAC            map[string]*macBucket
	mu                sync.Mutex
	lastRefill        time.Time
	refillInterval    time.Duration
}

type macBucket struct {
	tokens   int
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(enabled bool, globalLimit, perMACLimit int) *RateLimiter {
	if globalLimit <= 0 {
		globalLimit = 100
	}
	if perMACLimit <= 0 {
		perMACLimit = 10
	}
	return &RateLimiter{
		enabled:        enabled,
		globalLimit:    globalLimit,
		perMACLimit:    perMACLimit,
		globalTokens:   globalLimit,
		perMAC:         make(map[string]*macBucket),
		lastRefill:     time.Now(),
		refillInterval: time.Second,
	}
}

// Allow checks if a request from the given MAC is permitted.
// Returns true if allowed, false if rate-limited.
func (r *RateLimiter) Allow(mac net.HardwareAddr) bool {
	if !r.enabled {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.refill(now)

	// Check global limit
	if r.globalTokens <= 0 {
		return false
	}

	// Check per-MAC limit
	macStr := mac.String()
	bucket, exists := r.perMAC[macStr]
	if !exists {
		bucket = &macBucket{
			tokens:   r.perMACLimit,
			lastSeen: now,
		}
		r.perMAC[macStr] = bucket
	}

	if bucket.tokens <= 0 {
		return false
	}

	// Consume tokens
	r.globalTokens--
	bucket.tokens--
	bucket.lastSeen = now

	return true
}

// refill adds tokens back based on elapsed time since last refill.
func (r *RateLimiter) refill(now time.Time) {
	elapsed := now.Sub(r.lastRefill)
	if elapsed < r.refillInterval {
		return
	}

	// Refill proportional to elapsed time
	intervals := int(elapsed / r.refillInterval)
	if intervals <= 0 {
		return
	}

	r.lastRefill = now

	// Refill global tokens
	r.globalTokens += r.globalLimit * intervals
	if r.globalTokens > r.globalLimit {
		r.globalTokens = r.globalLimit
	}

	// Refill per-MAC tokens and clean up stale entries
	staleThreshold := 30 * time.Second
	for macStr, bucket := range r.perMAC {
		if now.Sub(bucket.lastSeen) > staleThreshold {
			delete(r.perMAC, macStr)
			continue
		}
		bucket.tokens += r.perMACLimit * intervals
		if bucket.tokens > r.perMACLimit {
			bucket.tokens = r.perMACLimit
		}
	}
}

// Stats returns current rate limiter statistics.
func (r *RateLimiter) Stats() (globalTokens int, trackedMACs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.globalTokens, len(r.perMAC)
}
