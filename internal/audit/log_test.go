package audit

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

func TestAuditAppendAndQuery(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	al, err := NewLog(db, bus, "node-1", testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Append some records directly
	now := time.Now().UTC()
	records := []Record{
		{Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), Event: "lease.ack", IP: "10.0.0.100", MAC: "aa:bb:cc:dd:ee:01", Subnet: "10.0.0.0/24", LeaseStart: now.Add(-2 * time.Hour).Unix(), LeaseExpiry: now.Add(22 * time.Hour).Unix()},
		{Timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339Nano), Event: "lease.renew", IP: "10.0.0.100", MAC: "aa:bb:cc:dd:ee:01", Subnet: "10.0.0.0/24", LeaseStart: now.Add(-1 * time.Hour).Unix(), LeaseExpiry: now.Add(23 * time.Hour).Unix()},
		{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339Nano), Event: "lease.ack", IP: "10.0.0.101", MAC: "aa:bb:cc:dd:ee:02", Subnet: "10.0.0.0/24", LeaseStart: now.Add(-30 * time.Minute).Unix(), LeaseExpiry: now.Add(23*time.Hour + 30*time.Minute).Unix()},
		{Timestamp: now.Format(time.RFC3339Nano), Event: "lease.release", IP: "10.0.0.100", MAC: "aa:bb:cc:dd:ee:01", Subnet: "10.0.0.0/24"},
	}
	for _, r := range records {
		if err := al.append(r); err != nil {
			t.Fatal(err)
		}
	}

	if al.Count() != 4 {
		t.Errorf("expected 4 records, got %d", al.Count())
	}

	// Query all
	all, err := al.Query(QueryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Errorf("query all: expected 4, got %d", len(all))
	}

	// Query by IP
	byIP, err := al.Query(QueryParams{IP: "10.0.0.100"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byIP) != 3 {
		t.Errorf("query by IP 10.0.0.100: expected 3, got %d", len(byIP))
	}

	// Query by MAC
	byMAC, err := al.Query(QueryParams{MAC: "aa:bb:cc:dd:ee:02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byMAC) != 1 {
		t.Errorf("query by MAC: expected 1, got %d", len(byMAC))
	}

	// Query by event type
	byEvent, err := al.Query(QueryParams{Event: "lease.ack"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byEvent) != 2 {
		t.Errorf("query by event lease.ack: expected 2, got %d", len(byEvent))
	}

	// Query by time range
	byRange, err := al.Query(QueryParams{
		From: now.Add(-90 * time.Minute),
		To:   now.Add(-15 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(byRange) != 2 {
		t.Errorf("query by time range: expected 2, got %d", len(byRange))
	}
}

func TestAuditPointInTimeQuery(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	al, err := NewLog(db, bus, "node-1", testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Simulate: device got 10.0.0.50 at 14:00, lease expires at 15:00
	t1 := time.Date(2025, 2, 15, 14, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 2, 15, 15, 0, 0, 0, time.UTC)

	al.append(Record{
		Timestamp:   t1.Format(time.RFC3339Nano),
		Event:       "lease.ack",
		IP:          "10.0.0.50",
		MAC:         "aa:bb:cc:dd:ee:ff",
		Hostname:    "device-1",
		Subnet:      "10.0.0.0/24",
		LeaseStart:  t1.Unix(),
		LeaseExpiry: t2.Unix(),
	})

	// Query: who had 10.0.0.50 at 14:30?
	queryTime := time.Date(2025, 2, 15, 14, 30, 0, 0, time.UTC)
	results, err := al.Query(QueryParams{
		IP: "10.0.0.50",
		At: queryTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("point-in-time query: expected 1, got %d", len(results))
	}
	if results[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected MAC aa:bb:cc:dd:ee:ff, got %s", results[0].MAC)
	}

	// Query: who had 10.0.0.50 at 15:30? (after expiry)
	afterExpiry := time.Date(2025, 2, 15, 15, 30, 0, 0, time.UTC)
	results2, err := al.Query(QueryParams{
		IP: "10.0.0.50",
		At: afterExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) != 0 {
		t.Errorf("after-expiry query: expected 0, got %d", len(results2))
	}
}

func TestAuditEventBusIntegration(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	al, err := NewLog(db, bus, "node-test", testLogger())
	if err != nil {
		t.Fatal(err)
	}

	go al.Start()
	defer al.Stop()

	// Give subscriber time to register
	time.Sleep(50 * time.Millisecond)

	// Publish a lease event
	bus.Publish(events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			IP:       net.ParseIP("192.168.1.10"),
			MAC:      net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01},
			Hostname: "test-device",
			Subnet:   "192.168.1.0/24",
			Start:    time.Now().Unix(),
			Expiry:   time.Now().Add(24 * time.Hour).Unix(),
		},
	})

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	results, err := al.Query(QueryParams{IP: "192.168.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 audit record from event bus, got %d", len(results))
	}
	if results[0].Hostname != "test-device" {
		t.Errorf("expected hostname test-device, got %s", results[0].Hostname)
	}
	if results[0].ServerID != "node-test" {
		t.Errorf("expected server_id node-test, got %s", results[0].ServerID)
	}
}

func TestAuditLimit(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	al, err := NewLog(db, bus, "node-1", testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Insert 20 records
	for i := 0; i < 20; i++ {
		al.append(Record{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			Event:     "lease.ack",
			IP:        "10.0.0.1",
			MAC:       "aa:bb:cc:dd:ee:ff",
		})
	}

	// Query with limit
	results, err := al.Query(QueryParams{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results with limit, got %d", len(results))
	}

	// Results should be newest first
	if results[0].ID < results[4].ID {
		t.Error("expected results ordered newest first")
	}
}

func TestAuditNonLeaseEventsIgnored(t *testing.T) {
	db := testDB(t)
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	al, err := NewLog(db, bus, "node-1", testLogger())
	if err != nil {
		t.Fatal(err)
	}

	go al.Start()
	defer al.Stop()

	time.Sleep(50 * time.Millisecond)

	// Publish a non-lease event (should be ignored)
	bus.Publish(events.Event{
		Type:      events.EventConflictDetected,
		Timestamp: time.Now(),
		Conflict: &events.ConflictData{
			IP:     net.ParseIP("10.0.0.1"),
			Subnet: "10.0.0.0/24",
		},
	})

	time.Sleep(200 * time.Millisecond)

	if al.Count() != 0 {
		t.Errorf("expected 0 audit records for non-lease event, got %d", al.Count())
	}
}
