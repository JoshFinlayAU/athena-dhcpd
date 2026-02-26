package rogue

import (
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	bolt "go.etcd.io/bbolt"
)

func testDB(t *testing.T) *bolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestDetectRogueServer(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	ch := bus.Subscribe(100)

	ownIPs := []net.IP{net.ParseIP("10.0.0.1")}
	d, err := NewDetector(db, bus, ownIPs, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Report our own server — should be ignored
	d.ReportOffer(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.50"), nil, nil, "eth0")
	if d.Count() != 0 {
		t.Errorf("own IP should be ignored, got count %d", d.Count())
	}

	// Report a rogue server
	d.ReportOffer(
		net.ParseIP("10.0.0.254"),
		net.ParseIP("10.0.0.100"),
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01},
		net.HardwareAddr{0xff, 0xff, 0xff, 0x00, 0x00, 0x01},
		"eth0",
	)

	if d.Count() != 1 {
		t.Errorf("expected 1 rogue server, got %d", d.Count())
	}

	all := d.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d entries", len(all))
	}
	if all[0].ServerIP != "10.0.0.254" {
		t.Errorf("server IP = %q", all[0].ServerIP)
	}
	if all[0].Count != 1 {
		t.Errorf("count = %d, want 1", all[0].Count)
	}

	// Check event was published
	select {
	case evt := <-ch:
		if evt.Type != events.EventRogueDetected {
			t.Errorf("event type = %q, want rogue.detected", evt.Type)
		}
		if evt.Rogue == nil {
			t.Fatal("rogue data is nil")
		}
		if evt.Rogue.ServerIP.String() != "10.0.0.254" {
			t.Errorf("rogue server IP = %q", evt.Rogue.ServerIP)
		}
	case <-time.After(time.Second):
		t.Error("no rogue event received")
	}
}

func TestRepeatedRogueIncrementsCount(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	d, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	rogueIP := net.ParseIP("192.168.1.254")
	d.ReportOffer(rogueIP, net.ParseIP("192.168.1.100"), nil, nil, "eth0")
	d.ReportOffer(rogueIP, net.ParseIP("192.168.1.101"), nil, nil, "eth0")
	d.ReportOffer(rogueIP, net.ParseIP("192.168.1.102"), nil, nil, "eth0")

	if d.Count() != 1 {
		t.Errorf("expected 1 rogue server, got %d", d.Count())
	}

	all := d.All()
	if all[0].Count != 3 {
		t.Errorf("count = %d, want 3", all[0].Count)
	}
	if all[0].LastOffer != "192.168.1.102" {
		t.Errorf("last offer = %q, want 192.168.1.102", all[0].LastOffer)
	}
}

func TestAcknowledge(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	d, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	d.ReportOffer(net.ParseIP("10.0.0.254"), nil, nil, nil, "eth0")

	if d.ActiveCount() != 1 {
		t.Errorf("active count = %d, want 1", d.ActiveCount())
	}

	if err := d.Acknowledge("10.0.0.254"); err != nil {
		t.Fatal(err)
	}

	if d.ActiveCount() != 0 {
		t.Errorf("active count after ack = %d, want 0", d.ActiveCount())
	}

	// Still exists
	if d.Count() != 1 {
		t.Errorf("total count = %d, want 1", d.Count())
	}
}

func TestRemoveRogue(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	ch := bus.Subscribe(100)

	d, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	d.ReportOffer(net.ParseIP("10.0.0.254"), nil, nil, nil, "eth0")
	// Drain the detected event
	<-ch

	if err := d.Remove("10.0.0.254"); err != nil {
		t.Fatal(err)
	}

	if d.Count() != 0 {
		t.Errorf("count after remove = %d, want 0", d.Count())
	}

	// Check resolved event
	select {
	case evt := <-ch:
		if evt.Type != events.EventRogueResolved {
			t.Errorf("event type = %q, want rogue.resolved", evt.Type)
		}
	case <-time.After(time.Second):
		t.Error("no resolved event")
	}

	// Remove non-existent should error
	if err := d.Remove("10.0.0.254"); err == nil {
		t.Error("expected error removing non-existent rogue")
	}
}

func TestPersistence(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	d1, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	d1.ReportOffer(net.ParseIP("10.0.0.254"), net.ParseIP("10.0.0.50"), nil, nil, "eth0")
	d1.ReportOffer(net.ParseIP("10.0.0.253"), nil, nil, nil, "eth1")

	// Create new detector from same DB
	d2, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if d2.Count() != 2 {
		t.Errorf("after reload: count = %d, want 2", d2.Count())
	}
}

func TestAddOwnIP(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	d, err := NewDetector(db, bus, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Report before adding as own
	d.ReportOffer(net.ParseIP("10.0.0.1"), nil, nil, nil, "eth0")
	if d.Count() != 1 {
		t.Fatal("expected rogue before adding own IP")
	}

	// Add as own IP
	d.AddOwnIP(net.ParseIP("10.0.0.1"))

	// New reports from this IP should be ignored
	d.ReportOffer(net.ParseIP("10.0.0.1"), nil, nil, nil, "eth0")
	all := d.All()
	for _, e := range all {
		if e.ServerIP == "10.0.0.1" && e.Count > 1 {
			t.Error("own IP should not increment count after AddOwnIP")
		}
	}
}
