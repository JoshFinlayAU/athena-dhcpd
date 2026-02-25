package conflict

import (
	"net"
	"testing"
	"time"
)

func TestProbeCacheMarkClear(t *testing.T) {
	cache := NewProbeCache(100 * time.Millisecond)
	ip := net.IPv4(192, 168, 1, 100)

	// Not in cache initially
	if cache.IsClear(ip) {
		t.Error("IP should not be clear initially")
	}

	cache.MarkClear(ip)
	if !cache.IsClear(ip) {
		t.Error("IP should be clear after MarkClear")
	}
	if cache.IsConflict(ip) {
		t.Error("IP should not be conflict after MarkClear")
	}
}

func TestProbeCacheMarkConflict(t *testing.T) {
	cache := NewProbeCache(100 * time.Millisecond)
	ip := net.IPv4(192, 168, 1, 100)

	cache.MarkConflict(ip)
	if cache.IsClear(ip) {
		t.Error("IP should not be clear after MarkConflict")
	}
	if !cache.IsConflict(ip) {
		t.Error("IP should be conflict after MarkConflict")
	}
}

func TestProbeCacheTTLExpiry(t *testing.T) {
	cache := NewProbeCache(10 * time.Millisecond)
	ip := net.IPv4(192, 168, 1, 100)

	cache.MarkClear(ip)
	if !cache.IsClear(ip) {
		t.Error("IP should be clear immediately after MarkClear")
	}

	time.Sleep(20 * time.Millisecond)

	if cache.IsClear(ip) {
		t.Error("IP should not be clear after TTL expiry")
	}
}

func TestProbeCacheInvalidate(t *testing.T) {
	cache := NewProbeCache(time.Hour)
	ip := net.IPv4(192, 168, 1, 100)

	cache.MarkClear(ip)
	if !cache.IsClear(ip) {
		t.Error("IP should be clear after MarkClear")
	}

	cache.Invalidate(ip)
	if cache.IsClear(ip) {
		t.Error("IP should not be clear after Invalidate")
	}
}

func TestProbeCacheOverwrite(t *testing.T) {
	cache := NewProbeCache(time.Hour)
	ip := net.IPv4(192, 168, 1, 100)

	cache.MarkClear(ip)
	if !cache.IsClear(ip) {
		t.Error("should be clear")
	}

	cache.MarkConflict(ip)
	if cache.IsClear(ip) {
		t.Error("should not be clear after overwrite with conflict")
	}
	if !cache.IsConflict(ip) {
		t.Error("should be conflict after overwrite")
	}
}

func TestProbeCacheCleanup(t *testing.T) {
	cache := NewProbeCache(10 * time.Millisecond)

	cache.MarkClear(net.IPv4(192, 168, 1, 100))
	cache.MarkClear(net.IPv4(192, 168, 1, 101))
	cache.MarkConflict(net.IPv4(192, 168, 1, 102))

	time.Sleep(20 * time.Millisecond)

	cache.Cleanup()

	// All entries should be gone
	if cache.IsClear(net.IPv4(192, 168, 1, 100)) {
		t.Error("entry should be cleaned up")
	}
	if cache.IsClear(net.IPv4(192, 168, 1, 101)) {
		t.Error("entry should be cleaned up")
	}
	if cache.IsConflict(net.IPv4(192, 168, 1, 102)) {
		t.Error("entry should be cleaned up")
	}
}
