package topology

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestRecordBuildsTopology(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m.Record(LeaseEvent{
		CircuitID: "eth0/1/3",
		RemoteID:  "switch-01",
		GIAddr:    "10.0.0.1",
		MAC:       "aa:bb:cc:dd:ee:01",
		IP:        "10.0.0.100",
		Hostname:  "device-1",
		Subnet:    "10.0.0.0/24",
	})

	stats := m.Stats()
	if stats["switches"] != 1 {
		t.Errorf("switches = %d, want 1", stats["switches"])
	}
	if stats["ports"] != 1 {
		t.Errorf("ports = %d, want 1", stats["ports"])
	}
	if stats["devices"] != 1 {
		t.Errorf("devices = %d, want 1", stats["devices"])
	}

	tree := m.Tree()
	if len(tree) != 1 {
		t.Fatalf("tree has %d switches", len(tree))
	}
	if tree[0].ID != "switch-01" {
		t.Errorf("switch ID = %q", tree[0].ID)
	}
	if tree[0].GIAddr != "10.0.0.1" {
		t.Errorf("giaddr = %q", tree[0].GIAddr)
	}
	port := tree[0].Ports["eth0/1/3"]
	if port == nil {
		t.Fatal("port eth0/1/3 not found")
	}
	if len(port.Devices) != 1 {
		t.Fatalf("port has %d devices", len(port.Devices))
	}
	if port.Devices[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("device MAC = %q", port.Devices[0].MAC)
	}
}

func TestMultipleDevicesOnPort(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m.Record(LeaseEvent{CircuitID: "eth0/1/1", RemoteID: "sw1", MAC: "aa:bb:cc:00:00:01", IP: "10.0.0.1"})
	m.Record(LeaseEvent{CircuitID: "eth0/1/1", RemoteID: "sw1", MAC: "aa:bb:cc:00:00:02", IP: "10.0.0.2"})
	m.Record(LeaseEvent{CircuitID: "eth0/1/2", RemoteID: "sw1", MAC: "aa:bb:cc:00:00:03", IP: "10.0.0.3"})

	stats := m.Stats()
	if stats["switches"] != 1 {
		t.Errorf("switches = %d, want 1", stats["switches"])
	}
	if stats["ports"] != 2 {
		t.Errorf("ports = %d, want 2", stats["ports"])
	}
	if stats["devices"] != 3 {
		t.Errorf("devices = %d, want 3", stats["devices"])
	}
}

func TestDeviceUpdateOnSameMAC(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m.Record(LeaseEvent{CircuitID: "port1", RemoteID: "sw1", MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.10", Hostname: "old"})
	m.Record(LeaseEvent{CircuitID: "port1", RemoteID: "sw1", MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.20", Hostname: "new"})

	tree := m.Tree()
	port := tree[0].Ports["port1"]
	if len(port.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(port.Devices))
	}
	if port.Devices[0].IP != "10.0.0.20" {
		t.Errorf("IP should be updated to 10.0.0.20, got %s", port.Devices[0].IP)
	}
	if port.Devices[0].Hostname != "new" {
		t.Errorf("hostname should be updated to 'new', got %s", port.Devices[0].Hostname)
	}
}

func TestMultipleSwitches(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m.Record(LeaseEvent{CircuitID: "port1", RemoteID: "switch-A", MAC: "aa:00:00:00:00:01", IP: "10.0.0.1"})
	m.Record(LeaseEvent{CircuitID: "port1", RemoteID: "switch-B", MAC: "aa:00:00:00:00:02", IP: "10.0.1.1"})

	if m.Stats()["switches"] != 2 {
		t.Errorf("expected 2 switches")
	}
}

func TestFallbackToGIAddr(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// No remote-id, only giaddr
	m.Record(LeaseEvent{CircuitID: "port1", GIAddr: "10.0.0.1", MAC: "aa:00:00:00:00:01", IP: "10.0.0.100"})

	tree := m.Tree()
	if len(tree) != 1 {
		t.Fatalf("expected 1 switch, got %d", len(tree))
	}
	if tree[0].ID != "10.0.0.1" {
		t.Errorf("switch ID should fall back to giaddr, got %q", tree[0].ID)
	}
}

func TestSetLabel(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m.Record(LeaseEvent{CircuitID: "eth0/1/1", RemoteID: "sw1", MAC: "aa:00:00:00:00:01", IP: "10.0.0.1"})

	if err := m.SetLabel("sw1", "", "Core Switch 1"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetLabel("sw1", "eth0/1/1", "Server Room Port 1"); err != nil {
		t.Fatal(err)
	}

	tree := m.Tree()
	if tree[0].Label != "Core Switch 1" {
		t.Errorf("switch label = %q", tree[0].Label)
	}
	if tree[0].Ports["eth0/1/1"].Label != "Server Room Port 1" {
		t.Errorf("port label = %q", tree[0].Ports["eth0/1/1"].Label)
	}

	// Invalid switch
	if err := m.SetLabel("nonexist", "", "x"); err == nil {
		t.Error("expected error for non-existent switch")
	}
}

func TestPersistence(t *testing.T) {
	db := testDB(t)
	m1, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	m1.Record(LeaseEvent{CircuitID: "port1", RemoteID: "sw1", MAC: "aa:00:00:00:00:01", IP: "10.0.0.1"})
	m1.Record(LeaseEvent{CircuitID: "port2", RemoteID: "sw1", MAC: "aa:00:00:00:00:02", IP: "10.0.0.2"})

	// Reload
	m2, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	stats := m2.Stats()
	if stats["switches"] != 1 || stats["ports"] != 2 || stats["devices"] != 2 {
		t.Errorf("after reload: %v", stats)
	}
}

func TestIgnoreNoOption82(t *testing.T) {
	db := testDB(t)
	m, err := NewMap(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// No circuit-id or remote-id — should be ignored
	m.Record(LeaseEvent{MAC: "aa:00:00:00:00:01", IP: "10.0.0.1"})

	if m.Stats()["switches"] != 0 {
		t.Error("should ignore events without option 82 data")
	}
}
