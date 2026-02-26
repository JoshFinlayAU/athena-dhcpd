package dbconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	bolt "go.etcd.io/bbolt"
)

func testDB(t *testing.T) *bolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewStore(t *testing.T) {
	db := testDB(t)
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if len(s.Subnets()) != 0 {
		t.Errorf("expected 0 subnets, got %d", len(s.Subnets()))
	}
}

func TestSubnetCRUD(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	sub := config.SubnetConfig{
		Network:    "192.168.1.0/24",
		Routers:    []string{"192.168.1.1"},
		DNSServers: []string{"8.8.8.8"},
		DomainName: "example.com",
		LeaseTime:  "1h",
		Pools: []config.PoolConfig{
			{RangeStart: "192.168.1.10", RangeEnd: "192.168.1.200"},
		},
	}

	// Create
	if err := s.PutSubnet(sub); err != nil {
		t.Fatalf("PutSubnet: %v", err)
	}
	if len(s.Subnets()) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(s.Subnets()))
	}

	// Read
	got, ok := s.GetSubnet("192.168.1.0/24")
	if !ok {
		t.Fatal("GetSubnet returned false")
	}
	if got.DomainName != "example.com" {
		t.Errorf("domain = %q, want example.com", got.DomainName)
	}

	// Update
	sub.DomainName = "updated.com"
	if err := s.PutSubnet(sub); err != nil {
		t.Fatalf("PutSubnet update: %v", err)
	}
	got, _ = s.GetSubnet("192.168.1.0/24")
	if got.DomainName != "updated.com" {
		t.Errorf("domain = %q, want updated.com", got.DomainName)
	}
	if len(s.Subnets()) != 1 {
		t.Errorf("expected 1 subnet after update, got %d", len(s.Subnets()))
	}

	// Delete
	if err := s.DeleteSubnet("192.168.1.0/24"); err != nil {
		t.Fatalf("DeleteSubnet: %v", err)
	}
	if len(s.Subnets()) != 0 {
		t.Errorf("expected 0 subnets after delete, got %d", len(s.Subnets()))
	}
}

func TestSubnetPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Write
	db, _ := bolt.Open(path, 0600, nil)
	s, _ := NewStore(db)
	s.PutSubnet(config.SubnetConfig{Network: "10.0.0.0/8", DomainName: "test.local"})
	db.Close()

	// Re-open
	db2, _ := bolt.Open(path, 0600, nil)
	defer db2.Close()
	s2, _ := NewStore(db2)
	subs := s2.Subnets()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subnet after reopen, got %d", len(subs))
	}
	if subs[0].DomainName != "test.local" {
		t.Errorf("domain = %q, want test.local", subs[0].DomainName)
	}
}

func TestReservationCRUD(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	s.PutSubnet(config.SubnetConfig{Network: "192.168.1.0/24"})

	res := config.ReservationConfig{MAC: "00:11:22:33:44:55", IP: "192.168.1.100", Hostname: "server1"}
	if err := s.PutReservation("192.168.1.0/24", res); err != nil {
		t.Fatalf("PutReservation: %v", err)
	}

	reservations := s.GetReservations("192.168.1.0/24")
	if len(reservations) != 1 {
		t.Fatalf("expected 1 reservation, got %d", len(reservations))
	}
	if reservations[0].Hostname != "server1" {
		t.Errorf("hostname = %q, want server1", reservations[0].Hostname)
	}

	// Update existing
	res.Hostname = "server1-updated"
	if err := s.PutReservation("192.168.1.0/24", res); err != nil {
		t.Fatalf("PutReservation update: %v", err)
	}
	reservations = s.GetReservations("192.168.1.0/24")
	if len(reservations) != 1 {
		t.Fatalf("expected 1 reservation after update, got %d", len(reservations))
	}
	if reservations[0].Hostname != "server1-updated" {
		t.Errorf("hostname = %q, want server1-updated", reservations[0].Hostname)
	}

	// Delete
	if err := s.DeleteReservation("192.168.1.0/24", "00:11:22:33:44:55"); err != nil {
		t.Fatalf("DeleteReservation: %v", err)
	}
	if len(s.GetReservations("192.168.1.0/24")) != 0 {
		t.Error("expected 0 reservations after delete")
	}
}

func TestImportReservations(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	s.PutSubnet(config.SubnetConfig{Network: "10.0.0.0/24"})

	batch := []config.ReservationConfig{
		{MAC: "aa:bb:cc:dd:ee:01", IP: "10.0.0.10", Hostname: "host1"},
		{MAC: "aa:bb:cc:dd:ee:02", IP: "10.0.0.11", Hostname: "host2"},
		{MAC: "aa:bb:cc:dd:ee:03", IP: "10.0.0.12", Hostname: "host3"},
	}

	added, err := s.ImportReservations("10.0.0.0/24", batch)
	if err != nil {
		t.Fatalf("ImportReservations: %v", err)
	}
	if added != 3 {
		t.Errorf("added = %d, want 3", added)
	}

	// Import again with overlap
	batch2 := []config.ReservationConfig{
		{MAC: "aa:bb:cc:dd:ee:01", IP: "10.0.0.10", Hostname: "host1-updated"},
		{MAC: "aa:bb:cc:dd:ee:04", IP: "10.0.0.13", Hostname: "host4"},
	}
	added2, err := s.ImportReservations("10.0.0.0/24", batch2)
	if err != nil {
		t.Fatalf("ImportReservations 2: %v", err)
	}
	if added2 != 1 {
		t.Errorf("added2 = %d, want 1", added2)
	}

	reservations := s.GetReservations("10.0.0.0/24")
	if len(reservations) != 4 {
		t.Errorf("expected 4 reservations, got %d", len(reservations))
	}
}

func TestSingletonConfigs(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	// Defaults
	d := config.DefaultsConfig{LeaseTime: "24h", DNSServers: []string{"1.1.1.1"}}
	if err := s.SetDefaults(d); err != nil {
		t.Fatalf("SetDefaults: %v", err)
	}
	if s.Defaults().LeaseTime != "24h" {
		t.Errorf("lease_time = %q, want 24h", s.Defaults().LeaseTime)
	}

	// Conflict detection
	c := config.ConflictDetectionConfig{Enabled: true, ProbeStrategy: "parallel"}
	if err := s.SetConflictDetection(c); err != nil {
		t.Fatalf("SetConflictDetection: %v", err)
	}
	if !s.ConflictDetection().Enabled {
		t.Error("expected conflict detection enabled")
	}

	// HA is bootstrap config (TOML only) — no DB getter/setter to test
}

func TestBuildConfig(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	s.PutSubnet(config.SubnetConfig{Network: "10.0.0.0/24"})
	s.SetDefaults(config.DefaultsConfig{LeaseTime: "2h"})
	s.SetConflictDetection(config.ConflictDetectionConfig{Enabled: true})

	bootstrap := &config.Config{
		Server: config.ServerConfig{Interface: "eth0", BindAddress: "0.0.0.0:67"},
		API:    config.APIConfig{Enabled: true, Listen: ":8080"},
	}

	full := s.BuildConfig(bootstrap)
	if full.Server.Interface != "eth0" {
		t.Error("bootstrap server config not preserved")
	}
	if len(full.Subnets) != 1 {
		t.Errorf("expected 1 subnet, got %d", len(full.Subnets))
	}
	if full.Defaults.LeaseTime != "2h" {
		t.Errorf("defaults lease_time = %q, want 2h", full.Defaults.LeaseTime)
	}
	if !full.ConflictDetection.Enabled {
		t.Error("expected conflict detection enabled in built config")
	}
}

func TestImportFromConfig(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	fullCfg := &config.Config{
		Subnets: []config.SubnetConfig{
			{Network: "192.168.1.0/24", DomainName: "a.com"},
			{Network: "10.0.0.0/8", DomainName: "b.com"},
		},
		Defaults:          config.DefaultsConfig{LeaseTime: "6h"},
		ConflictDetection: config.ConflictDetectionConfig{Enabled: true},
		HA:                config.HAConfig{Role: "secondary"},
		DNS:               config.DNSProxyConfig{Enabled: true, Domain: "local"},
	}

	if err := s.ImportFromConfig(fullCfg); err != nil {
		t.Fatalf("ImportFromConfig: %v", err)
	}

	if len(s.Subnets()) != 2 {
		t.Errorf("expected 2 subnets, got %d", len(s.Subnets()))
	}
	if s.Defaults().LeaseTime != "6h" {
		t.Errorf("defaults = %q, want 6h", s.Defaults().LeaseTime)
	}
}

func TestV1ImportedFlag(t *testing.T) {
	db := testDB(t)
	s, _ := NewStore(db)

	if s.IsV1Imported() {
		t.Error("should not be marked imported initially")
	}
	s.MarkV1Imported()
	if !s.IsV1Imported() {
		t.Error("should be marked imported after MarkV1Imported")
	}
}

// Suppress unused import warning
var _ = os.TempDir
