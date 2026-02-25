package lease

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewStore(t *testing.T) {
	store := newTestStore(t)
	if store.Count() != 0 {
		t.Errorf("Count() = %d, want 0", store.Count())
	}
}

func TestStorePutAndGet(t *testing.T) {
	store := newTestStore(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ip := net.IPv4(192, 168, 1, 100)
	now := time.Now()

	l := &Lease{
		IP:          ip,
		MAC:         mac,
		ClientID:    "client1",
		Hostname:    "testhost",
		Subnet:      "192.168.1.0/24",
		Pool:        "192.168.1.100-192.168.1.200",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(8 * time.Hour),
		LastUpdated: now,
		UpdateSeq:   1,
	}

	if err := store.Put(l); err != nil {
		t.Fatalf("Put error: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("Count() = %d, want 1", store.Count())
	}

	// Get by IP
	got := store.GetByIP(ip)
	if got == nil {
		t.Fatal("GetByIP returned nil")
	}
	if !got.IP.Equal(ip) {
		t.Errorf("IP = %s, want %s", got.IP, ip)
	}
	if got.MAC.String() != mac.String() {
		t.Errorf("MAC = %s, want %s", got.MAC, mac)
	}
	if got.Hostname != "testhost" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "testhost")
	}

	// Get by MAC
	got2 := store.GetByMAC(mac)
	if got2 == nil {
		t.Fatal("GetByMAC returned nil")
	}
	if !got2.IP.Equal(ip) {
		t.Errorf("GetByMAC IP = %s, want %s", got2.IP, ip)
	}

	// Get by ClientID
	got3 := store.GetByClientID("client1")
	if got3 == nil {
		t.Fatal("GetByClientID returned nil")
	}

	// Get by Hostname
	got4 := store.GetByHostname("testhost")
	if got4 == nil {
		t.Fatal("GetByHostname returned nil")
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	store := newTestStore(t)

	if got := store.GetByIP(net.IPv4(10, 0, 0, 1)); got != nil {
		t.Errorf("GetByIP for non-existent IP = %v, want nil", got)
	}

	mac, _ := net.ParseMAC("ff:ff:ff:ff:ff:ff")
	if got := store.GetByMAC(mac); got != nil {
		t.Errorf("GetByMAC for non-existent MAC = %v, want nil", got)
	}

	if got := store.GetByClientID("nonexistent"); got != nil {
		t.Errorf("GetByClientID for non-existent = %v, want nil", got)
	}
}

func TestStoreDelete(t *testing.T) {
	store := newTestStore(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ip := net.IPv4(192, 168, 1, 100)
	now := time.Now()

	l := &Lease{
		IP:          ip,
		MAC:         mac,
		ClientID:    "client1",
		Hostname:    "testhost",
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	}

	store.Put(l)

	if err := store.Delete(ip); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	if store.Count() != 0 {
		t.Errorf("Count after delete = %d, want 0", store.Count())
	}

	if got := store.GetByIP(ip); got != nil {
		t.Error("GetByIP should return nil after delete")
	}
	if got := store.GetByMAC(mac); got != nil {
		t.Error("GetByMAC should return nil after delete")
	}
	if got := store.GetByClientID("client1"); got != nil {
		t.Error("GetByClientID should return nil after delete")
	}
}

func TestStoreDeleteNonExistent(t *testing.T) {
	store := newTestStore(t)
	// Should not error
	if err := store.Delete(net.IPv4(10, 0, 0, 1)); err != nil {
		t.Errorf("Delete non-existent: %v", err)
	}
}

func TestStoreUpdateLease(t *testing.T) {
	store := newTestStore(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ip := net.IPv4(192, 168, 1, 100)
	now := time.Now()

	l := &Lease{
		IP:          ip,
		MAC:         mac,
		Hostname:    "host1",
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateOffered,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	}
	store.Put(l)

	// Update state to active
	l.State = dhcpv4.LeaseStateActive
	l.Hostname = "host1-updated"
	l.LastUpdated = time.Now()
	store.Put(l)

	got := store.GetByIP(ip)
	if got == nil {
		t.Fatal("GetByIP returned nil after update")
	}
	if got.State != dhcpv4.LeaseStateActive {
		t.Errorf("State = %q, want %q", got.State, dhcpv4.LeaseStateActive)
	}
	if got.Hostname != "host1-updated" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "host1-updated")
	}

	// Count should still be 1
	if store.Count() != 1 {
		t.Errorf("Count = %d after update, want 1", store.Count())
	}
}

func TestStoreAll(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for i := 0; i < 5; i++ {
		mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, byte(i)}
		ip := net.IPv4(192, 168, 1, byte(100+i))
		store.Put(&Lease{
			IP:          ip,
			MAC:         mac,
			Subnet:      "192.168.1.0/24",
			State:       dhcpv4.LeaseStateActive,
			Start:       now,
			Expiry:      now.Add(time.Hour),
			LastUpdated: now,
		})
	}

	all := store.All()
	if len(all) != 5 {
		t.Errorf("All() = %d, want 5", len(all))
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Create store and add a lease
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ip := net.IPv4(192, 168, 1, 100)
	now := time.Now()

	store.Put(&Lease{
		IP:          ip,
		MAC:         mac,
		Hostname:    "persistent",
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	})
	store.Close()

	// Reopen
	store2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen error: %v", err)
	}
	defer store2.Close()

	if store2.Count() != 1 {
		t.Errorf("Count after reopen = %d, want 1", store2.Count())
	}

	got := store2.GetByIP(ip)
	if got == nil {
		t.Fatal("GetByIP after reopen returned nil")
	}
	if got.Hostname != "persistent" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "persistent")
	}
	if got.MAC.String() != mac.String() {
		t.Errorf("MAC = %s, want %s", got.MAC, mac)
	}
}

func TestStoreNextSeq(t *testing.T) {
	store := newTestStore(t)

	s1 := store.NextSeq()
	s2 := store.NextSeq()
	s3 := store.NextSeq()

	if s1 != 1 || s2 != 2 || s3 != 3 {
		t.Errorf("NextSeq() sequence = %d, %d, %d, want 1, 2, 3", s1, s2, s3)
	}
}

func TestStoreForEach(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for i := 0; i < 3; i++ {
		mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, byte(i)}
		ip := net.IPv4(192, 168, 1, byte(100+i))
		store.Put(&Lease{
			IP:          ip,
			MAC:         mac,
			Subnet:      "192.168.1.0/24",
			State:       dhcpv4.LeaseStateActive,
			Start:       now,
			Expiry:      now.Add(time.Hour),
			LastUpdated: now,
		})
	}

	count := 0
	store.ForEach(func(l *Lease) bool {
		count++
		return true
	})
	if count != 3 {
		t.Errorf("ForEach visited %d leases, want 3", count)
	}

	// Test early termination
	count = 0
	store.ForEach(func(l *Lease) bool {
		count++
		return false // Stop after first
	})
	if count != 1 {
		t.Errorf("ForEach with early stop visited %d leases, want 1", count)
	}
}
