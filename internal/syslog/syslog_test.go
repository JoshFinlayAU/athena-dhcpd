package syslog

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestFormatLeaseEvent(t *testing.T) {
	evt := events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       net.ParseIP("10.0.0.50"),
			MAC:      net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01},
			Hostname: "laptop-1",
			Subnet:   "10.0.0.0/24",
		},
	}

	msg := FormatMessage(evt)
	if !strings.Contains(msg, "event=lease.ack") {
		t.Errorf("missing event type in %q", msg)
	}
	if !strings.Contains(msg, "ip=10.0.0.50") {
		t.Errorf("missing IP in %q", msg)
	}
	if !strings.Contains(msg, "mac=aa:bb:cc:dd:ee:01") {
		t.Errorf("missing MAC in %q", msg)
	}
	if !strings.Contains(msg, "hostname=laptop-1") {
		t.Errorf("missing hostname in %q", msg)
	}
	if !strings.Contains(msg, "subnet=10.0.0.0/24") {
		t.Errorf("missing subnet in %q", msg)
	}
}

func TestFormatConflictEvent(t *testing.T) {
	evt := events.Event{
		Type:      events.EventConflictDetected,
		Timestamp: time.Now(),
		Conflict: &events.ConflictData{
			IP:              net.ParseIP("10.0.0.100"),
			DetectionMethod: "arp",
		},
	}

	msg := FormatMessage(evt)
	if !strings.Contains(msg, "conflict_ip=10.0.0.100") {
		t.Errorf("missing conflict IP in %q", msg)
	}
	if !strings.Contains(msg, "method=arp") {
		t.Errorf("missing method in %q", msg)
	}
}

func TestFormatRogueEvent(t *testing.T) {
	evt := events.Event{
		Type:      events.EventRogueDetected,
		Timestamp: time.Now(),
		Rogue: &events.RogueData{
			ServerIP: net.ParseIP("10.0.0.254"),
			Count:    5,
		},
	}

	msg := FormatMessage(evt)
	if !strings.Contains(msg, "rogue_server=10.0.0.254") {
		t.Errorf("missing rogue server in %q", msg)
	}
	if !strings.Contains(msg, "count=5") {
		t.Errorf("missing count in %q", msg)
	}
}

func TestEventSeverity(t *testing.T) {
	tests := []struct {
		evtType  events.EventType
		severity int
	}{
		{events.EventLeaseAck, SeverityInfo},
		{events.EventRogueDetected, SeverityWarning},
		{events.EventConflictDetected, SeverityWarning},
		{events.EventAnomalyDetected, SeverityWarning},
		{events.EventLeaseDecline, SeverityNotice},
		{events.EventHAFailover, SeverityNotice},
	}

	for _, tc := range tests {
		got := eventSeverity(tc.evtType)
		if got != tc.severity {
			t.Errorf("eventSeverity(%s) = %d, want %d", tc.evtType, got, tc.severity)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Protocol != "udp" {
		t.Errorf("protocol = %q", cfg.Protocol)
	}
	if cfg.Facility != FacilityLocal0 {
		t.Errorf("facility = %d", cfg.Facility)
	}
	if cfg.Tag != "athena-dhcpd" {
		t.Errorf("tag = %q", cfg.Tag)
	}
}

func TestForwarderUDP(t *testing.T) {
	// Start a UDP listener to receive syslog messages
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := Config{
		Address:  conn.LocalAddr().String(),
		Protocol: "udp",
		Facility: FacilityLocal0,
		Tag:      "test",
	}

	fwd := NewForwarder(cfg, bus, testLogger())
	if err := fwd.Start(); err != nil {
		t.Fatal(err)
	}
	defer fwd.Stop()

	// Publish an event
	bus.Publish(events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:     net.ParseIP("10.0.0.50"),
			MAC:    net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01},
			Subnet: "10.0.0.0/24",
		},
	})

	// Read from UDP
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("no syslog message received: %v", err)
	}

	msg := string(buf[:n])
	if !strings.Contains(msg, "event=lease.ack") {
		t.Errorf("syslog message missing event: %q", msg)
	}
	if !strings.Contains(msg, "ip=10.0.0.50") {
		t.Errorf("syslog message missing IP: %q", msg)
	}
	if !strings.Contains(msg, "test") {
		t.Errorf("syslog message missing tag: %q", msg)
	}
}

func TestForwarderNoAddress(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	fwd := NewForwarder(Config{}, bus, testLogger())
	if err := fwd.Start(); err == nil {
		t.Error("expected error with empty address")
		fwd.Stop()
	}
}
