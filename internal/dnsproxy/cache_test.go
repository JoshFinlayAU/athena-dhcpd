package dnsproxy

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func makeTestMsg(name string, qtype uint16, ttl uint32) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   []byte{10, 0, 0, 1},
		},
	}
	msg.Rcode = dns.RcodeSuccess
	return msg
}

func TestCacheSetAndGet(t *testing.T) {
	c := NewCache(100)

	msg := makeTestMsg("host.example.com", dns.TypeA, 300)
	c.Set(msg, 5*time.Minute)

	got := c.Get("host.example.com.", dns.TypeA, dns.ClassINET)
	if got == nil {
		t.Fatal("expected cached response, got nil")
	}
	if len(got.Answer) != 1 {
		t.Errorf("cached response has %d answers, want 1", len(got.Answer))
	}
}

func TestCacheMiss(t *testing.T) {
	c := NewCache(100)

	got := c.Get("nonexistent.example.com.", dns.TypeA, dns.ClassINET)
	if got != nil {
		t.Errorf("expected nil for cache miss, got %v", got)
	}
}

func TestCacheExpiry(t *testing.T) {
	c := NewCache(100)

	msg := makeTestMsg("host.example.com", dns.TypeA, 1)

	// Manually set with very short TTL
	c.mu.Lock()
	c.entries[cacheKey("host.example.com.", dns.TypeA, dns.ClassINET)] = &cacheEntry{
		msg:       msg.Copy(),
		expiresAt: time.Now().Add(-1 * time.Second), // already expired
	}
	c.mu.Unlock()

	got := c.Get("host.example.com.", dns.TypeA, dns.ClassINET)
	if got != nil {
		t.Error("expected nil for expired entry")
	}
}

func TestCacheTTLFromAnswer(t *testing.T) {
	c := NewCache(100)

	msg := makeTestMsg("host.example.com", dns.TypeA, 10) // TTL=10s
	c.Set(msg, 5*time.Minute)                              // default 5min

	if c.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", c.Size())
	}

	// Entry should exist and use the answer TTL (10s), not the default (5min)
	c.mu.RLock()
	entry := c.entries[cacheKey("host.example.com.", dns.TypeA, dns.ClassINET)]
	c.mu.RUnlock()

	if entry == nil {
		t.Fatal("entry not found")
	}

	// TTL should be ~10s, not ~5min
	remaining := time.Until(entry.expiresAt)
	if remaining > 15*time.Second {
		t.Errorf("TTL should be ~10s from answer, got %v", remaining)
	}
}

func TestCacheErrorTTLCapped(t *testing.T) {
	c := NewCache(100)

	msg := new(dns.Msg)
	msg.SetQuestion("nonexistent.example.com.", dns.TypeA)
	msg.Rcode = dns.RcodeNameError // NXDOMAIN

	c.Set(msg, 5*time.Minute)

	c.mu.RLock()
	entry := c.entries[cacheKey("nonexistent.example.com.", dns.TypeA, dns.ClassINET)]
	c.mu.RUnlock()

	if entry == nil {
		t.Fatal("error entry not cached")
	}

	remaining := time.Until(entry.expiresAt)
	if remaining > 35*time.Second {
		t.Errorf("error TTL should be capped at 30s, got %v", remaining)
	}
}

func TestCacheFlush(t *testing.T) {
	c := NewCache(100)

	c.Set(makeTestMsg("a.example.com", dns.TypeA, 300), 5*time.Minute)
	c.Set(makeTestMsg("b.example.com", dns.TypeA, 300), 5*time.Minute)

	if c.Size() != 2 {
		t.Fatalf("Size() = %d before flush, want 2", c.Size())
	}

	c.Flush()
	if c.Size() != 0 {
		t.Errorf("Size() = %d after flush, want 0", c.Size())
	}
}

func TestCacheEviction(t *testing.T) {
	c := NewCache(3)

	c.Set(makeTestMsg("a.example.com", dns.TypeA, 300), 5*time.Minute)
	c.Set(makeTestMsg("b.example.com", dns.TypeA, 300), 5*time.Minute)
	c.Set(makeTestMsg("c.example.com", dns.TypeA, 300), 5*time.Minute)

	// At capacity, adding one more should trigger eviction
	c.Set(makeTestMsg("d.example.com", dns.TypeA, 300), 5*time.Minute)

	if c.Size() > 3 {
		t.Errorf("cache should not exceed maxSize, got %d", c.Size())
	}
}

func TestCacheNilMsg(t *testing.T) {
	c := NewCache(100)
	c.Set(nil, 5*time.Minute) // should not panic
	if c.Size() != 0 {
		t.Error("setting nil msg should be a no-op")
	}
}

func TestCacheNoQuestion(t *testing.T) {
	c := NewCache(100)
	msg := new(dns.Msg) // no question
	c.Set(msg, 5*time.Minute)
	if c.Size() != 0 {
		t.Error("setting msg with no question should be a no-op")
	}
}

func TestCacheReturnsCopy(t *testing.T) {
	c := NewCache(100)
	msg := makeTestMsg("host.example.com", dns.TypeA, 300)
	c.Set(msg, 5*time.Minute)

	got1 := c.Get("host.example.com.", dns.TypeA, dns.ClassINET)
	got2 := c.Get("host.example.com.", dns.TypeA, dns.ClassINET)

	if got1 == got2 {
		t.Error("Get should return copies, not the same pointer")
	}
}

func TestNewCacheDefaultSize(t *testing.T) {
	c := NewCache(0)
	if c.maxSize != 10000 {
		t.Errorf("default maxSize = %d, want 10000", c.maxSize)
	}

	c = NewCache(-5)
	if c.maxSize != 10000 {
		t.Errorf("negative maxSize should default to 10000, got %d", c.maxSize)
	}
}
