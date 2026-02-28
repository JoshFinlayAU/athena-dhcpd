package ddns

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// mockUpdater records DNS update calls for testing.
type mockUpdater struct {
	mu         sync.Mutex
	aAdded     []string
	aRemoved   []string
	ptrAdded   []string
	ptrRemoved []string
	failNext   bool
}

func (m *mockUpdater) AddA(zone, fqdn string, ip net.IP, ttl uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return fmt.Errorf("mock AddA failure")
	}
	m.aAdded = append(m.aAdded, fqdn)
	return nil
}

func (m *mockUpdater) RemoveA(zone, fqdn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aRemoved = append(m.aRemoved, fqdn)
	return nil
}

func (m *mockUpdater) AddPTR(zone, reverseIP, fqdn string, ttl uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return fmt.Errorf("mock AddPTR failure")
	}
	m.ptrAdded = append(m.ptrAdded, reverseIP)
	return nil
}

func (m *mockUpdater) RemovePTR(zone, reverseIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ptrRemoved = append(m.ptrRemoved, reverseIP)
	return nil
}

func newTestManager(t *testing.T) (*Manager, *mockUpdater, *mockUpdater, *events.Bus) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := events.NewBus(100, logger)
	go bus.Start()
	t.Cleanup(func() { bus.Stop() })

	fwd := &mockUpdater{}
	rev := &mockUpdater{}

	cfg := &config.DDNSConfig{
		Enabled:         true,
		AllowClientFQDN: true,
		FallbackToMAC:   true,
		TTL:             300,
		Forward: config.DDNSZoneConfig{
			Zone:   "example.com.",
			Method: "rfc2136",
		},
		Reverse: config.DDNSZoneConfig{
			Zone:   "1.168.192.in-addr.arpa.",
			Method: "rfc2136",
		},
	}

	mgr := NewManagerForTest(cfg, bus, logger, fwd, rev)
	return mgr, fwd, rev, bus
}

func TestManagerFQDNConstruction(t *testing.T) {
	mgr, _, _, _ := newTestManager(t)
	mac := "00:11:22:33:44:55"

	tests := []struct {
		name  string
		lease *events.LeaseData
		want  string
	}{
		{
			"hostname + domain",
			&events.LeaseData{Hostname: "myhost", MAC: mac},
			"myhost.example.com.",
		},
		{
			"client FQDN",
			&events.LeaseData{FQDN: "custom.example.org", Hostname: "myhost", MAC: mac},
			"custom.example.org.",
		},
		{
			"MAC fallback",
			&events.LeaseData{MAC: mac},
			"00-11-22-33-44-55.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mgr.GetFQDNForLease(tt.lease)
			if got != tt.want {
				t.Errorf("FQDN = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManagerAddRecords(t *testing.T) {
	mgr, fwd, rev, bus := newTestManager(t)

	mac := "00:11:22:33:44:55"
	ip := net.IPv4(192, 168, 1, 100)

	evt := events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       ip,
			MAC:      mac,
			Hostname: "testhost",
			Subnet:   "192.168.1.0/24",
		},
	}

	mgr.handleEvent(evt)
	mgr.wg.Wait()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	if len(fwd.aAdded) != 1 {
		t.Fatalf("expected 1 A record added, got %d", len(fwd.aAdded))
	}
	if fwd.aAdded[0] != "testhost.example.com." {
		t.Errorf("A record FQDN = %q, want %q", fwd.aAdded[0], "testhost.example.com.")
	}

	rev.mu.Lock()
	defer rev.mu.Unlock()
	if len(rev.ptrAdded) != 1 {
		t.Fatalf("expected 1 PTR record added, got %d", len(rev.ptrAdded))
	}

	_ = bus // keep reference
}

func TestManagerRemoveRecords(t *testing.T) {
	mgr, fwd, rev, _ := newTestManager(t)

	mac := "00:11:22:33:44:55"
	ip := net.IPv4(192, 168, 1, 100)

	evt := events.Event{
		Type:      events.EventLeaseRelease,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       ip,
			MAC:      mac,
			Hostname: "testhost",
			Subnet:   "192.168.1.0/24",
		},
	}

	mgr.handleEvent(evt)
	mgr.wg.Wait()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	if len(fwd.aRemoved) != 1 {
		t.Fatalf("expected 1 A record removed, got %d", len(fwd.aRemoved))
	}

	rev.mu.Lock()
	defer rev.mu.Unlock()
	if len(rev.ptrRemoved) != 1 {
		t.Fatalf("expected 1 PTR record removed, got %d", len(rev.ptrRemoved))
	}
}

func TestManagerSkipsRenewByDefault(t *testing.T) {
	mgr, fwd, _, _ := newTestManager(t)

	mac := "00:11:22:33:44:55"
	ip := net.IPv4(192, 168, 1, 100)

	evt := events.Event{
		Type:      events.EventLeaseRenew,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       ip,
			MAC:      mac,
			Hostname: "testhost",
			Subnet:   "192.168.1.0/24",
		},
	}

	mgr.handleEvent(evt)
	mgr.wg.Wait()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	if len(fwd.aAdded) != 0 {
		t.Errorf("expected 0 A records on renew (update_on_renew=false), got %d", len(fwd.aAdded))
	}
}

func TestManagerUpdatesOnRenewWhenEnabled(t *testing.T) {
	mgr, fwd, _, _ := newTestManager(t)
	mgr.cfg.UpdateOnRenew = true

	mac := "00:11:22:33:44:55"
	ip := net.IPv4(192, 168, 1, 100)

	evt := events.Event{
		Type:      events.EventLeaseRenew,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       ip,
			MAC:      mac,
			Hostname: "testhost",
			Subnet:   "192.168.1.0/24",
		},
	}

	mgr.handleEvent(evt)
	mgr.wg.Wait()

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	if len(fwd.aAdded) != 1 {
		t.Errorf("expected 1 A record on renew (update_on_renew=true), got %d", len(fwd.aAdded))
	}
}

func TestManagerZoneOverride(t *testing.T) {
	mgr, _, _, _ := newTestManager(t)
	mgr.cfg.ZoneOverrides = []config.DDNSZoneOverride{
		{Subnet: "10.0.0.0/24", ForwardZone: "lab.example.com.", ReverseZone: "0.0.10.in-addr.arpa."},
	}

	if got := mgr.getForwardZone("10.0.0.0/24"); got != "lab.example.com." {
		t.Errorf("forward zone for 10.0.0.0/24 = %q, want %q", got, "lab.example.com.")
	}
	if got := mgr.getReverseZone("10.0.0.0/24"); got != "0.0.10.in-addr.arpa." {
		t.Errorf("reverse zone for 10.0.0.0/24 = %q, want %q", got, "0.0.10.in-addr.arpa.")
	}

	// Non-overridden subnet uses default
	if got := mgr.getForwardZone("192.168.1.0/24"); got != "example.com." {
		t.Errorf("forward zone for 192.168.1.0/24 = %q, want %q", got, "example.com.")
	}
}
