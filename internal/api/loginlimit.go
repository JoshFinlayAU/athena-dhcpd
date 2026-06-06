package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginLimiter throttles failed login attempts per client to slow online
// password brute-forcing. After maxFailures within window, the client is locked
// out for lockout. Successful logins reset the counter.
type loginLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*loginAttempts
	maxFailures int
	window      time.Duration
	lockout     time.Duration
}

type loginAttempts struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func newLoginLimiter(maxFailures int, window, lockout time.Duration) *loginLimiter {
	return &loginLimiter{
		attempts:    make(map[string]*loginAttempts),
		maxFailures: maxFailures,
		window:      window,
		lockout:     lockout,
	}
}

// locked reports whether the client is currently locked out and, if so, how long
// remains. It also opportunistically prunes stale entries.
func (l *loginLimiter) locked(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok {
		return false, 0
	}
	if now.Before(a.lockedUntil) {
		return true, a.lockedUntil.Sub(now)
	}
	// Lock expired or never set; drop entries whose window has elapsed.
	if a.lockedUntil.IsZero() && now.Sub(a.windowStart) > l.window {
		delete(l.attempts, key)
	}
	return false, 0
}

// recordFailure increments the failure count for a client, starting a lockout
// once maxFailures is reached within the window.
func (l *loginLimiter) recordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok || now.Sub(a.windowStart) > l.window {
		l.attempts[key] = &loginAttempts{count: 1, windowStart: now}
		return
	}
	a.count++
	if a.count >= l.maxFailures {
		a.lockedUntil = now.Add(l.lockout)
		a.count = 0
		a.windowStart = now
	}
}

// reset clears any recorded failures/lockout for a client (called on success).
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// clientIP extracts the client IP from the request's remote address. It does not
// trust X-Forwarded-For, which is spoofable unless a trusted proxy strips it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
