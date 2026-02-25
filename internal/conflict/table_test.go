package conflict

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(path)
	})
	return db
}

func TestNewTable(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}
	if table.Count() != 0 {
		t.Errorf("Count() = %d, want 0", table.Count())
	}
}

func TestTableAddAndIsConflicted(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip := net.IPv4(192, 168, 1, 100)

	// Not conflicted initially
	if table.IsConflicted(ip) {
		t.Error("IP should not be conflicted initially")
	}

	// Add conflict
	permanent, err := table.Add(ip, "arp_probe", "aa:bb:cc:dd:ee:ff", "192.168.1.0/24")
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if permanent {
		t.Error("should not be permanent after 1 conflict")
	}

	// Now conflicted
	if !table.IsConflicted(ip) {
		t.Error("IP should be conflicted after Add")
	}

	// Check record
	r := table.Get(ip)
	if r == nil {
		t.Fatal("Get returned nil")
	}
	if r.ProbeCount != 1 {
		t.Errorf("ProbeCount = %d, want 1", r.ProbeCount)
	}
	if r.DetectionMethod != "arp_probe" {
		t.Errorf("DetectionMethod = %q, want %q", r.DetectionMethod, "arp_probe")
	}
	if r.Subnet != "192.168.1.0/24" {
		t.Errorf("Subnet = %q, want %q", r.Subnet, "192.168.1.0/24")
	}
}

func TestTablePermanentFlag(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3) // max_conflict_count = 3
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip := net.IPv4(192, 168, 1, 100)

	// Add 3 conflicts → should become permanent
	for i := 0; i < 2; i++ {
		permanent, err := table.Add(ip, "arp_probe", "", "192.168.1.0/24")
		if err != nil {
			t.Fatalf("Add error at %d: %v", i, err)
		}
		if permanent {
			t.Errorf("should not be permanent at count %d", i+1)
		}
	}

	// Third conflict → permanent
	permanent, err := table.Add(ip, "arp_probe", "", "192.168.1.0/24")
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if !permanent {
		t.Error("should be permanent after 3 conflicts")
	}

	r := table.Get(ip)
	if r == nil {
		t.Fatal("Get returned nil")
	}
	if !r.Permanent {
		t.Error("record should be marked permanent")
	}
	if r.ProbeCount != 3 {
		t.Errorf("ProbeCount = %d, want 3", r.ProbeCount)
	}
}

func TestTableResolve(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip := net.IPv4(192, 168, 1, 100)
	table.Add(ip, "arp_probe", "", "192.168.1.0/24")

	if !table.IsConflicted(ip) {
		t.Error("should be conflicted after Add")
	}

	if err := table.Resolve(ip); err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if table.IsConflicted(ip) {
		t.Error("should not be conflicted after Resolve")
	}

	r := table.Get(ip)
	if r == nil {
		t.Fatal("Get returned nil")
	}
	if !r.Resolved {
		t.Error("record should be marked resolved")
	}
}

func TestTableClear(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip := net.IPv4(192, 168, 1, 100)
	table.Add(ip, "arp_probe", "", "192.168.1.0/24")

	if err := table.Clear(ip); err != nil {
		t.Fatalf("Clear error: %v", err)
	}

	r := table.Get(ip)
	if r != nil {
		t.Error("record should be nil after Clear")
	}
	if table.IsConflicted(ip) {
		t.Error("should not be conflicted after Clear")
	}
}

func TestTableHoldTimeExpiry(t *testing.T) {
	db := newTestDB(t)
	// Very short hold time
	table, err := NewTable(db, 10*time.Millisecond, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip := net.IPv4(192, 168, 1, 100)
	table.Add(ip, "arp_probe", "", "192.168.1.0/24")

	if !table.IsConflicted(ip) {
		t.Error("should be conflicted immediately after Add")
	}

	// Wait for hold time to expire
	time.Sleep(20 * time.Millisecond)

	if table.IsConflicted(ip) {
		t.Error("should not be conflicted after hold time expired")
	}
}

func TestTableCleanupExpired(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, 10*time.Millisecond, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	ip1 := net.IPv4(192, 168, 1, 100)
	ip2 := net.IPv4(192, 168, 1, 101)
	table.Add(ip1, "arp_probe", "", "192.168.1.0/24")
	table.Add(ip2, "arp_probe", "", "192.168.1.0/24")

	time.Sleep(20 * time.Millisecond)

	resolved := table.CleanupExpired()
	if len(resolved) != 2 {
		t.Errorf("CleanupExpired resolved %d, want 2", len(resolved))
	}
}

func TestTableAllActive(t *testing.T) {
	db := newTestDB(t)
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	table.Add(net.IPv4(192, 168, 1, 100), "arp_probe", "", "192.168.1.0/24")
	table.Add(net.IPv4(192, 168, 1, 101), "icmp_probe", "", "192.168.1.0/24")

	active := table.AllActive()
	if len(active) != 2 {
		t.Errorf("AllActive() = %d, want 2", len(active))
	}
}

func TestTablePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create table and add a conflict
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	table, err := NewTable(db, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}
	ip := net.IPv4(192, 168, 1, 100)
	table.Add(ip, "arp_probe", "aa:bb:cc:dd:ee:ff", "192.168.1.0/24")
	db.Close()

	// Reopen and verify persistence
	db2, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()

	table2, err := NewTable(db2, time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable 2 error: %v", err)
	}

	if !table2.IsConflicted(ip) {
		t.Error("conflict should persist after DB reopen")
	}

	r := table2.Get(ip)
	if r == nil {
		t.Fatal("record should exist after reopen")
	}
	if r.ResponderMAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("ResponderMAC = %q, want %q", r.ResponderMAC, "aa:bb:cc:dd:ee:ff")
	}
}
