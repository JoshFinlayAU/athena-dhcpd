package pool

import (
	"net"
	"testing"
)

func newTestPool(t *testing.T) *Pool {
	t.Helper()
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	p, err := NewPool("test", net.IPv4(192, 168, 1, 100), net.IPv4(192, 168, 1, 110), network)
	if err != nil {
		t.Fatalf("NewPool error: %v", err)
	}
	return p
}

func TestNewPool(t *testing.T) {
	p := newTestPool(t)
	if p.Size() != 11 {
		t.Errorf("Size() = %d, want 11", p.Size())
	}
	if p.Allocated() != 0 {
		t.Errorf("Allocated() = %d, want 0", p.Allocated())
	}
	if p.Available() != 11 {
		t.Errorf("Available() = %d, want 11", p.Available())
	}
}

func TestNewPoolInvalidRange(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")

	// End before start
	_, err := NewPool("bad", net.IPv4(192, 168, 1, 200), net.IPv4(192, 168, 1, 100), network)
	if err == nil {
		t.Error("expected error for end before start")
	}

	// Start not in network
	_, err = NewPool("bad", net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 10), network)
	if err == nil {
		t.Error("expected error for start not in network")
	}
}

func TestPoolAllocate(t *testing.T) {
	p := newTestPool(t)

	// Allocate first IP
	ip := p.Allocate()
	if ip == nil {
		t.Fatal("Allocate() returned nil")
	}
	if !ip.Equal(net.IPv4(192, 168, 1, 100)) {
		t.Errorf("first allocation = %s, want 192.168.1.100", ip)
	}
	if p.Allocated() != 1 {
		t.Errorf("Allocated() = %d, want 1", p.Allocated())
	}

	// Allocate second IP
	ip2 := p.Allocate()
	if ip2 == nil {
		t.Fatal("second Allocate() returned nil")
	}
	if ip2.Equal(ip) {
		t.Error("second allocation should be different from first")
	}
}

func TestPoolAllocateAll(t *testing.T) {
	p := newTestPool(t)

	// Allocate all 11 IPs
	allocated := make(map[string]bool)
	for i := 0; i < 11; i++ {
		ip := p.Allocate()
		if ip == nil {
			t.Fatalf("Allocate() returned nil at iteration %d", i)
		}
		ipStr := ip.String()
		if allocated[ipStr] {
			t.Fatalf("duplicate allocation: %s", ipStr)
		}
		allocated[ipStr] = true
	}

	if p.Available() != 0 {
		t.Errorf("Available() = %d, want 0 after allocating all", p.Available())
	}

	// Next allocation should return nil (pool exhausted)
	if ip := p.Allocate(); ip != nil {
		t.Errorf("Allocate() on full pool = %s, want nil", ip)
	}
}

func TestPoolAllocateSpecific(t *testing.T) {
	p := newTestPool(t)

	ip := net.IPv4(192, 168, 1, 105)
	if !p.AllocateSpecific(ip) {
		t.Error("AllocateSpecific(105) should succeed")
	}
	if p.Allocated() != 1 {
		t.Errorf("Allocated() = %d, want 1", p.Allocated())
	}

	// Allocate same IP again — should fail
	if p.AllocateSpecific(ip) {
		t.Error("AllocateSpecific(105) should fail (already allocated)")
	}

	// Out of range
	if p.AllocateSpecific(net.IPv4(192, 168, 1, 50)) {
		t.Error("AllocateSpecific out of range should fail")
	}
}

func TestPoolRelease(t *testing.T) {
	p := newTestPool(t)

	ip := p.Allocate()
	if ip == nil {
		t.Fatal("Allocate() returned nil")
	}

	if !p.Release(ip) {
		t.Error("Release() should succeed for allocated IP")
	}
	if p.Allocated() != 0 {
		t.Errorf("Allocated() = %d after release, want 0", p.Allocated())
	}

	// Release same IP again — should fail
	if p.Release(ip) {
		t.Error("Release() should fail for already-released IP")
	}

	// Release out of range
	if p.Release(net.IPv4(10, 0, 0, 1)) {
		t.Error("Release() out of range should fail")
	}
}

func TestPoolContains(t *testing.T) {
	p := newTestPool(t)

	if !p.Contains(net.IPv4(192, 168, 1, 100)) {
		t.Error("Contains(100) should be true")
	}
	if !p.Contains(net.IPv4(192, 168, 1, 110)) {
		t.Error("Contains(110) should be true")
	}
	if p.Contains(net.IPv4(192, 168, 1, 99)) {
		t.Error("Contains(99) should be false")
	}
	if p.Contains(net.IPv4(192, 168, 1, 111)) {
		t.Error("Contains(111) should be false")
	}
	if p.Contains(net.IPv4(10, 0, 0, 1)) {
		t.Error("Contains(10.0.0.1) should be false")
	}
}

func TestPoolIsAllocated(t *testing.T) {
	p := newTestPool(t)

	ip := net.IPv4(192, 168, 1, 105)
	if p.IsAllocated(ip) {
		t.Error("IsAllocated should be false before allocation")
	}
	p.AllocateSpecific(ip)
	if !p.IsAllocated(ip) {
		t.Error("IsAllocated should be true after allocation")
	}
}

func TestPoolAllocateN(t *testing.T) {
	p := newTestPool(t)

	ips := p.AllocateN(3)
	if len(ips) != 3 {
		t.Fatalf("AllocateN(3) returned %d IPs, want 3", len(ips))
	}

	// All should be different
	seen := make(map[string]bool)
	for _, ip := range ips {
		if seen[ip.String()] {
			t.Errorf("duplicate in AllocateN: %s", ip)
		}
		seen[ip.String()] = true
	}

	// All should be in range
	for _, ip := range ips {
		if !p.Contains(ip) {
			t.Errorf("AllocateN returned IP %s not in pool", ip)
		}
	}
}

func TestPoolAllocateNMoreThanAvailable(t *testing.T) {
	p := newTestPool(t)

	// Pool has 11 IPs, ask for 20
	ips := p.AllocateN(20)
	if len(ips) != 11 {
		t.Errorf("AllocateN(20) returned %d IPs, want 11 (pool size)", len(ips))
	}
}

func TestPoolUtilization(t *testing.T) {
	p := newTestPool(t)

	if p.Utilization() != 0 {
		t.Errorf("Utilization() = %f, want 0", p.Utilization())
	}

	p.Allocate()
	// 1/11 ≈ 9.09%
	util := p.Utilization()
	if util < 9.0 || util > 9.2 {
		t.Errorf("Utilization() after 1 allocation = %f, want ~9.09", util)
	}
}

func TestPoolString(t *testing.T) {
	p := newTestPool(t)
	s := p.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}
