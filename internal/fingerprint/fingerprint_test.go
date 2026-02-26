package fingerprint

import (
	"log/slog"
	"net"
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

func mustMAC(s string) net.HardwareAddr {
	m, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func TestFingerprintHash(t *testing.T) {
	fp1 := &RawFingerprint{
		VendorClass: "MSFT 5.0",
		ParamList:   []byte{1, 3, 6, 15, 44, 46, 47},
	}
	fp2 := &RawFingerprint{
		VendorClass: "MSFT 5.0",
		ParamList:   []byte{1, 3, 6, 15, 44, 46, 47},
	}
	fp3 := &RawFingerprint{
		VendorClass: "android-dhcp-12",
		ParamList:   []byte{1, 3, 6, 15, 26, 28},
	}

	if fp1.Hash() != fp2.Hash() {
		t.Error("identical fingerprints should have same hash")
	}
	if fp1.Hash() == fp3.Hash() {
		t.Error("different fingerprints should have different hashes")
	}
}

func TestParamListString(t *testing.T) {
	fp := &RawFingerprint{ParamList: []byte{1, 3, 6, 15}}
	got := fp.ParamListString()
	if got != "1,3,6,15" {
		t.Errorf("ParamListString() = %q, want %q", got, "1,3,6,15")
	}

	fp2 := &RawFingerprint{ParamList: nil}
	if fp2.ParamListString() != "" {
		t.Error("empty param list should return empty string")
	}
}

func TestClassifyWindows(t *testing.T) {
	info := &DeviceInfo{}
	fp := &RawFingerprint{
		VendorClass: "MSFT 5.0",
		ParamList:   []byte{1, 3, 6, 15},
	}
	classify(info, fp)

	if info.OS != "Windows" {
		t.Errorf("OS = %q, want Windows", info.OS)
	}
	if info.DeviceType != "computer" {
		t.Errorf("DeviceType = %q, want computer", info.DeviceType)
	}
	if info.DeviceName != "Windows 2000/XP" {
		t.Errorf("DeviceName = %q, want Windows 2000/XP", info.DeviceName)
	}
}

func TestClassifyAndroid(t *testing.T) {
	info := &DeviceInfo{}
	fp := &RawFingerprint{
		VendorClass: "android-dhcp-12",
		ParamList:   []byte{1, 3, 6, 15, 26, 28},
	}
	classify(info, fp)

	if info.OS != "Android" {
		t.Errorf("OS = %q, want Android", info.OS)
	}
	if info.DeviceType != "phone" {
		t.Errorf("DeviceType = %q, want phone", info.DeviceType)
	}
}

func TestClassifyHostname(t *testing.T) {
	tests := []struct {
		hostname string
		wantType string
		wantOS   string
	}{
		{"iPhone", "phone", "iOS/iPadOS"},
		{"MacBook-Pro", "computer", "macOS"},
		{"HP-LaserJet", "printer", ""},
		{"android-abc123def", "phone", "Android"},
		{"random-device", "unknown", ""},
	}

	for _, tt := range tests {
		info := &DeviceInfo{}
		fp := &RawFingerprint{Hostname: tt.hostname}
		classify(info, fp)
		if info.DeviceType != tt.wantType {
			t.Errorf("hostname %q: DeviceType = %q, want %q", tt.hostname, info.DeviceType, tt.wantType)
		}
		if tt.wantOS != "" && info.OS != tt.wantOS {
			t.Errorf("hostname %q: OS = %q, want %q", tt.hostname, info.OS, tt.wantOS)
		}
	}
}

func TestClassifyNetworkDevices(t *testing.T) {
	tests := []struct {
		vendorClass string
		wantName    string
	}{
		{"Cisco AP", "Cisco"},
		{"Aruba IAP", "Aruba"},
		{"Meraki MR", "Meraki"},
		{"ubnt-unifi", "Ubiquiti"},
	}

	for _, tt := range tests {
		info := &DeviceInfo{}
		fp := &RawFingerprint{VendorClass: tt.vendorClass}
		classify(info, fp)
		if info.DeviceType != "network" {
			t.Errorf("vendor %q: DeviceType = %q, want network", tt.vendorClass, info.DeviceType)
		}
		if info.DeviceName != tt.wantName {
			t.Errorf("vendor %q: DeviceName = %q, want %q", tt.vendorClass, info.DeviceName, tt.wantName)
		}
	}
}

func TestStoreRecordAndGet(t *testing.T) {
	db := testDB(t)
	store, err := NewStore(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	mac := mustMAC("aa:bb:cc:dd:ee:01")
	fp := &RawFingerprint{
		MAC:         mac,
		VendorClass: "MSFT 5.0",
		ParamList:   []byte{1, 3, 6, 15},
		Hostname:    "DESKTOP-ABC",
	}

	info := store.Record(fp)
	if info.OS != "Windows" {
		t.Errorf("OS = %q, want Windows", info.OS)
	}
	if info.Hostname != "DESKTOP-ABC" {
		t.Errorf("Hostname = %q, want DESKTOP-ABC", info.Hostname)
	}

	// Get should return the same info
	got := store.Get(mac.String())
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.FingerprintHash != info.FingerprintHash {
		t.Error("hash mismatch")
	}

	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}
}

func TestStoreUpdateLastSeen(t *testing.T) {
	db := testDB(t)
	store, err := NewStore(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	mac := mustMAC("aa:bb:cc:dd:ee:02")
	fp := &RawFingerprint{
		MAC:         mac,
		VendorClass: "android-dhcp-12",
		ParamList:   []byte{1, 3, 6},
		Hostname:    "Galaxy-S24",
	}

	info1 := store.Record(fp)
	time.Sleep(50 * time.Millisecond)

	// Record again with same fingerprint
	fp.Hostname = "Galaxy-S24-updated"
	info2 := store.Record(fp)

	if info2.FirstSeen != info1.FirstSeen {
		t.Error("FirstSeen should not change on update")
	}
	if !info2.LastSeen.After(info1.LastSeen) {
		t.Error("LastSeen should be updated")
	}
	if info2.Hostname != "Galaxy-S24-updated" {
		t.Errorf("Hostname not updated: %q", info2.Hostname)
	}

	// Still only 1 device
	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}
}

func TestStorePersistence(t *testing.T) {
	db := testDB(t)

	// Store 1: write
	store1, err := NewStore(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	store1.Record(&RawFingerprint{
		MAC:         mustMAC("aa:bb:cc:dd:ee:03"),
		VendorClass: "MSFT 10.0",
		ParamList:   []byte{1, 3, 6, 15},
	})

	// Store 2: reload from same DB
	store2, err := NewStore(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if store2.Count() != 1 {
		t.Errorf("after reload: Count = %d, want 1", store2.Count())
	}
	info := store2.Get("aa:bb:cc:dd:ee:03")
	if info == nil {
		t.Fatal("device not found after reload")
	}
	if info.OS != "Windows" {
		t.Errorf("OS after reload = %q, want Windows", info.OS)
	}
}

func TestStoreAll(t *testing.T) {
	db := testDB(t)
	store, err := NewStore(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	store.Record(&RawFingerprint{MAC: mustMAC("aa:bb:cc:dd:ee:01"), VendorClass: "MSFT 5.0"})
	time.Sleep(5 * time.Millisecond)
	store.Record(&RawFingerprint{MAC: mustMAC("aa:bb:cc:dd:ee:02"), VendorClass: "android-dhcp-12"})

	all := store.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d, want 2", len(all))
	}

	// Should be sorted newest first
	if all[0].LastSeen.Before(all[1].LastSeen) {
		t.Error("expected All() sorted newest first")
	}
}

func TestOUIExtraction(t *testing.T) {
	mac := mustMAC("aa:bb:cc:dd:ee:ff")
	oui := ouiFromMAC(mac)
	if oui != "aa:bb:cc" {
		t.Errorf("ouiFromMAC = %q, want aa:bb:cc", oui)
	}
}
