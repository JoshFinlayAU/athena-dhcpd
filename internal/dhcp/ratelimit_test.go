package dhcp

import (
	"net"
	"testing"
	"time"
)

func TestRateLimiterDisabled(t *testing.T) {
	rl := NewRateLimiter(false, 10, 5)
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !rl.Allow(mac) {
			t.Fatalf("disabled rate limiter rejected request %d", i)
		}
	}
}

func TestRateLimiterGlobalLimit(t *testing.T) {
	rl := NewRateLimiter(true, 5, 100) // 5 global, 100 per-MAC
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

	// First 5 should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow(mac) {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 6th should be rejected
	if rl.Allow(mac) {
		t.Error("6th request should be rejected (global limit)")
	}
}

func TestRateLimiterPerMACLimit(t *testing.T) {
	rl := NewRateLimiter(true, 100, 3) // 100 global, 3 per-MAC
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

	// First 3 should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow(mac) {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 4th from same MAC should be rejected
	if rl.Allow(mac) {
		t.Error("4th request from same MAC should be rejected")
	}

	// Different MAC should still be allowed
	mac2 := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if !rl.Allow(mac2) {
		t.Error("different MAC should still be allowed")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(true, 3, 3)
	rl.refillInterval = 50 * time.Millisecond // Speed up for testing
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

	// Exhaust tokens
	for i := 0; i < 3; i++ {
		rl.Allow(mac)
	}
	if rl.Allow(mac) {
		t.Error("should be rate-limited after exhausting tokens")
	}

	// Wait for refill
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow(mac) {
		t.Error("should be allowed after refill")
	}
}

func TestRateLimiterStats(t *testing.T) {
	rl := NewRateLimiter(true, 10, 5)

	mac1 := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	mac2 := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}

	rl.Allow(mac1)
	rl.Allow(mac2)

	tokens, macs := rl.Stats()
	if tokens != 8 { // 10 - 2
		t.Errorf("globalTokens = %d, want 8", tokens)
	}
	if macs != 2 {
		t.Errorf("trackedMACs = %d, want 2", macs)
	}
}
