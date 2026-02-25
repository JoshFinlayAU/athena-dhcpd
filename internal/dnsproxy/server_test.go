package dnsproxy

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/miekg/dns"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testConfig() *config.DNSProxyConfig {
	return &config.DNSProxyConfig{
		Enabled:          true,
		ListenUDP:        "127.0.0.1:15353",
		Domain:           "test.local",
		TTL:              60,
		RegisterLeases:   true,
		ForwardLeasesPTR: true,
		Forwarders:       []string{"8.8.8.8"},
		CacheSize:        100,
		CacheTTL:         "5m",
	}
}

func TestNewServer(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.zone == nil {
		t.Error("zone is nil")
	}
	if s.cache == nil {
		t.Error("cache is nil")
	}
	if s.Zone().Domain() != "test.local." {
		t.Errorf("domain = %q, want %q", s.Zone().Domain(), "test.local.")
	}
}

func TestNewServerWithStaticRecords(t *testing.T) {
	cfg := testConfig()
	cfg.StaticRecords = []config.DNSStaticRecord{
		{Name: "static.test.local", Type: "A", Value: "10.0.0.99", TTL: 120},
		{Name: "alias.test.local", Type: "CNAME", Value: "static.test.local", TTL: 0},
	}

	s := NewServer(cfg, testLogger())
	if s.Zone().Count() != 2 {
		t.Errorf("zone count = %d, want 2", s.Zone().Count())
	}

	rrs := s.Zone().Lookup("static.test.local.", dns.TypeA)
	if len(rrs) != 1 {
		t.Fatalf("static A record not found, got %d records", len(rrs))
	}
	aRec := rrs[0].(*dns.A)
	if !aRec.A.Equal(net.ParseIP("10.0.0.99")) {
		t.Errorf("static A = %s, want 10.0.0.99", aRec.A)
	}
	if aRec.Hdr.Ttl != 120 {
		t.Errorf("static A TTL = %d, want 120", aRec.Hdr.Ttl)
	}
}

func TestNewServerInvalidStaticRecord(t *testing.T) {
	cfg := testConfig()
	cfg.StaticRecords = []config.DNSStaticRecord{
		{Name: "bad.test.local", Type: "A", Value: "not-an-ip"},
	}

	// Should not panic, just skip the bad record
	s := NewServer(cfg, testLogger())
	if s.Zone().Count() != 0 {
		t.Errorf("bad static record should be skipped, zone count = %d", s.Zone().Count())
	}
}

func TestNewServerWithZoneOverrides(t *testing.T) {
	cfg := testConfig()
	cfg.ZoneOverrides = []config.DNSZoneOverride{
		{Zone: "internal.corp", Nameserver: "10.0.0.53"},
		{Zone: "CLOUD.EXAMPLE.COM", Nameserver: "10.0.0.54"},
	}

	s := NewServer(cfg, testLogger())
	if len(s.zoneOverrides) != 2 {
		t.Fatalf("zoneOverrides count = %d, want 2", len(s.zoneOverrides))
	}

	// Verify case normalization
	if _, ok := s.zoneOverrides["internal.corp."]; !ok {
		t.Error("zone override for 'internal.corp.' not found")
	}
	if _, ok := s.zoneOverrides["cloud.example.com."]; !ok {
		t.Error("zone override for 'cloud.example.com.' not found (case normalized)")
	}
}

func TestServerStartStop(t *testing.T) {
	cfg := testConfig()
	cfg.ListenUDP = "127.0.0.1:0" // ephemeral port
	s := NewServer(cfg, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should error
	if err := s.Start(ctx); err == nil {
		t.Error("expected error on double start")
	}

	s.Stop()

	// Double stop should be safe
	s.Stop()
}

func TestServerRegisterUnregisterLease(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	ip := net.ParseIP("192.168.1.10")
	s.RegisterLease("workstation1", ip)

	if !s.Zone().Has("workstation1.test.local.", dns.TypeA) {
		t.Error("A record not created after RegisterLease")
	}
	if !s.Zone().Has("10.1.168.192.in-addr.arpa.", dns.TypePTR) {
		t.Error("PTR record not created after RegisterLease")
	}

	s.UnregisterLease("workstation1", ip)

	if s.Zone().Has("workstation1.test.local.", dns.TypeA) {
		t.Error("A record still exists after UnregisterLease")
	}
	if s.Zone().Has("10.1.168.192.in-addr.arpa.", dns.TypePTR) {
		t.Error("PTR record still exists after UnregisterLease")
	}
}

func TestServerRegisterLeaseDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.RegisterLeases = false
	s := NewServer(cfg, testLogger())

	s.RegisterLease("host1", net.ParseIP("10.0.0.1"))
	if s.Zone().Count() != 0 {
		t.Error("should not register leases when disabled")
	}
}

func TestServerFlushCache(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	msg := makeTestMsg("cached.test.local", dns.TypeA, 300)
	s.Cache().Set(msg, 5*time.Minute)

	if s.Cache().Size() != 1 {
		t.Fatal("cache should have 1 entry")
	}

	s.FlushCache()
	if s.Cache().Size() != 0 {
		t.Error("cache should be empty after flush")
	}
}

func TestServerStats(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	stats := s.Stats()
	if stats["domain"] != "test.local" {
		t.Errorf("stats domain = %v, want test.local", stats["domain"])
	}
	if stats["forwarders"].(int) != 1 {
		t.Errorf("stats forwarders = %v, want 1", stats["forwarders"])
	}
}

func TestServerHandleQueryLocalZone(t *testing.T) {
	cfg := testConfig()
	cfg.ListenUDP = "127.0.0.1:25353"
	s := NewServer(cfg, testLogger())

	// Add a record to the zone
	a := &dns.A{
		Hdr: dns.RR_Header{Name: "myhost.test.local.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.42").To4(),
	}
	s.Zone().Add(a)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()

	// Give listener time to bind
	time.Sleep(100 * time.Millisecond)

	// Query the server
	client := new(dns.Client)
	msg := new(dns.Msg)
	msg.SetQuestion("myhost.test.local.", dns.TypeA)

	resp, _, err := client.Exchange(msg, "127.0.0.1:25353")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want success", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("got %d answers, want 1", len(resp.Answer))
	}

	aResp, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("expected A record in answer")
	}
	if !aResp.A.Equal(net.ParseIP("10.0.0.42")) {
		t.Errorf("answer IP = %s, want 10.0.0.42", aResp.A)
	}
	if !resp.Authoritative {
		t.Error("local zone responses should be authoritative")
	}
}

func TestFindZoneOverride(t *testing.T) {
	cfg := testConfig()
	cfg.ZoneOverrides = []config.DNSZoneOverride{
		{Zone: "corp.example.com", Nameserver: "10.0.0.53"},
		{Zone: "example.com", Nameserver: "10.0.0.54"},
	}

	s := NewServer(cfg, testLogger())

	tests := []struct {
		qname string
		want  string
		found bool
	}{
		{"host.corp.example.com.", "10.0.0.53", true},
		{"deep.host.corp.example.com.", "10.0.0.53", true},
		{"other.example.com.", "10.0.0.54", true},
		{"example.com.", "10.0.0.54", true},
		{"unrelated.org.", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.qname, func(t *testing.T) {
			zo, found := s.findZoneOverride(tt.qname)
			if found != tt.found {
				t.Errorf("found = %v, want %v", found, tt.found)
				return
			}
			if found && zo.Nameserver != tt.want {
				t.Errorf("nameserver = %q, want %q", zo.Nameserver, tt.want)
			}
		})
	}
}

func TestDecodeBase64URL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"simple", "aGVsbG8", "hello", false},
		{"with padding chars in url encoding", "aGVsbG8gd29ybGQ", "hello world", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64URL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestServerEventSubscription(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan events.Event, 10)
	s.SubscribeToEvents(ctx, ch)

	// Send a lease.ack event
	ch <- events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       net.ParseIP("10.0.0.50"),
			Hostname: "eventhost",
		},
	}

	// Give event handler time to process
	time.Sleep(50 * time.Millisecond)

	if !s.Zone().Has("eventhost.test.local.", dns.TypeA) {
		t.Error("lease.ack event should register A record")
	}

	// Send a release event
	ch <- events.Event{
		Type:      events.EventLeaseRelease,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       net.ParseIP("10.0.0.50"),
			Hostname: "eventhost",
		},
	}

	time.Sleep(50 * time.Millisecond)

	if s.Zone().Has("eventhost.test.local.", dns.TypeA) {
		t.Error("lease.release event should unregister A record")
	}
}

func TestServerEventSubscriptionNilLease(t *testing.T) {
	cfg := testConfig()
	s := NewServer(cfg, testLogger())

	// handleEvent with nil lease should not panic
	s.handleEvent(events.Event{
		Type: events.EventConflictDetected,
	})
}

func TestServerCacheTTLParseFallback(t *testing.T) {
	cfg := testConfig()
	cfg.CacheTTL = "invalid-duration"
	s := NewServer(cfg, testLogger())

	if s.cacheTTL != 5*time.Minute {
		t.Errorf("cacheTTL fallback = %v, want 5m", s.cacheTTL)
	}
}

func TestDohResponseWriter(t *testing.T) {
	w := &dohResponseWriter{}

	msg := new(dns.Msg)
	msg.SetQuestion("test.example.com.", dns.TypeA)

	if err := w.WriteMsg(msg); err != nil {
		t.Fatalf("WriteMsg failed: %v", err)
	}
	if w.msg == nil {
		t.Error("msg should be set after WriteMsg")
	}

	// Test Write (raw bytes)
	w2 := &dohResponseWriter{}
	packed, _ := msg.Pack()
	n, err := w2.Write(packed)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(packed) {
		t.Errorf("Write returned %d, want %d", n, len(packed))
	}
	if w2.msg == nil {
		t.Error("msg should be set after Write")
	}

	// Test interface methods don't panic
	w.Close()
	w.TsigStatus()
	w.TsigTimersOnly(true)
	w.Hijack()
	w.LocalAddr()
	w.RemoteAddr()
}
