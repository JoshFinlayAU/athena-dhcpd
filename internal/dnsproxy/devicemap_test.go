package dnsproxy

import (
	"net"
	"testing"
)

func TestDeviceMapperUpdateAndLookup(t *testing.T) {
	m := NewDeviceMapper()

	m.Update(net.ParseIP("10.0.0.5"), "aa:bb:cc:dd:ee:01", "laptop-1", "computer")

	dev := m.Lookup("10.0.0.5")
	if dev == nil {
		t.Fatal("expected device for 10.0.0.5")
	}
	if dev.MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("MAC = %q", dev.MAC)
	}
	if dev.Hostname != "laptop-1" {
		t.Errorf("Hostname = %q", dev.Hostname)
	}
	if dev.Type != "computer" {
		t.Errorf("Type = %q", dev.Type)
	}
}

func TestDeviceMapperLookupWithPort(t *testing.T) {
	m := NewDeviceMapper()
	m.Update(net.ParseIP("10.0.0.5"), "aa:bb:cc:dd:ee:01", "laptop-1", "computer")

	// DNS queries come with source port
	dev := m.Lookup("10.0.0.5:12345")
	if dev == nil {
		t.Fatal("should strip port and find device")
	}
	if dev.MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("MAC = %q", dev.MAC)
	}
}

func TestDeviceMapperRemove(t *testing.T) {
	m := NewDeviceMapper()
	m.Update(net.ParseIP("10.0.0.5"), "aa:bb:cc:dd:ee:01", "laptop-1", "computer")

	m.Remove(net.ParseIP("10.0.0.5"))

	if m.Lookup("10.0.0.5") != nil {
		t.Error("device should be removed")
	}
	if m.Count() != 0 {
		t.Errorf("count = %d, want 0", m.Count())
	}
}

func TestDeviceMapperEnrichEntry(t *testing.T) {
	m := NewDeviceMapper()
	m.Update(net.ParseIP("192.168.1.10"), "aa:bb:cc:00:00:01", "phone-1", "phone")

	entry := &QueryLogEntry{
		Source: "192.168.1.10:5353",
		Name:   "google.com.",
	}
	m.EnrichEntry(entry)

	if entry.DeviceMAC != "aa:bb:cc:00:00:01" {
		t.Errorf("DeviceMAC = %q", entry.DeviceMAC)
	}
	if entry.DeviceHostname != "phone-1" {
		t.Errorf("DeviceHostname = %q", entry.DeviceHostname)
	}
	if entry.DeviceType != "phone" {
		t.Errorf("DeviceType = %q", entry.DeviceType)
	}
}

func TestDeviceMapperEnrichUnknown(t *testing.T) {
	m := NewDeviceMapper()

	entry := &QueryLogEntry{
		Source: "192.168.1.99:5353",
		Name:   "google.com.",
	}
	m.EnrichEntry(entry)

	if entry.DeviceMAC != "" {
		t.Error("unknown device should not be enriched")
	}
}

func TestDeviceMapperAll(t *testing.T) {
	m := NewDeviceMapper()
	m.Update(net.ParseIP("10.0.0.1"), "aa:00:00:00:00:01", "dev1", "computer")
	m.Update(net.ParseIP("10.0.0.2"), "aa:00:00:00:00:02", "dev2", "phone")

	all := m.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d entries", len(all))
	}
}

func TestDeviceMapperUpdate(t *testing.T) {
	m := NewDeviceMapper()
	m.Update(net.ParseIP("10.0.0.5"), "aa:bb:cc:dd:ee:01", "old-name", "computer")
	m.Update(net.ParseIP("10.0.0.5"), "aa:bb:cc:dd:ee:01", "new-name", "computer")

	dev := m.Lookup("10.0.0.5")
	if dev.Hostname != "new-name" {
		t.Errorf("hostname should be updated, got %q", dev.Hostname)
	}
	if m.Count() != 1 {
		t.Errorf("count = %d, want 1", m.Count())
	}
}
