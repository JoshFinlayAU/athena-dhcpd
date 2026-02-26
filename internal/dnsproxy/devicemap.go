package dnsproxy

import (
	"net"
	"sync"
	"time"
)

// DeviceInfo holds identity info for a device on the network.
type DeviceInfo struct {
	MAC      string
	Hostname string
	Type     string // from fingerprinting
}

// DeviceMapper maps IP addresses to device identity info.
// It is populated from DHCP lease data.
type DeviceMapper struct {
	mu      sync.RWMutex
	devices map[string]*DeviceInfo // IP string -> device info
}

// NewDeviceMapper creates a new device mapper.
func NewDeviceMapper() *DeviceMapper {
	return &DeviceMapper{
		devices: make(map[string]*DeviceInfo),
	}
}

// Update adds or updates the device mapping for an IP.
func (m *DeviceMapper) Update(ip net.IP, mac, hostname, deviceType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[ip.String()] = &DeviceInfo{
		MAC:      mac,
		Hostname: hostname,
		Type:     deviceType,
	}
}

// Remove removes the device mapping for an IP.
func (m *DeviceMapper) Remove(ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.devices, ip.String())
}

// Lookup returns device info for an IP, or nil if unknown.
func (m *DeviceMapper) Lookup(ip string) *DeviceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Strip port if present
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		host = ip
	}

	return m.devices[host]
}

// Count returns how many devices are mapped.
func (m *DeviceMapper) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// All returns all current mappings (for API display).
func (m *DeviceMapper) All() map[string]DeviceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]DeviceInfo, len(m.devices))
	for k, v := range m.devices {
		result[k] = *v
	}
	return result
}

// EnrichEntry enriches a query log entry with device info based on source IP.
func (m *DeviceMapper) EnrichEntry(entry *QueryLogEntry) {
	dev := m.Lookup(entry.Source)
	if dev == nil {
		return
	}
	entry.DeviceMAC = dev.MAC
	entry.DeviceHostname = dev.Hostname
	entry.DeviceType = dev.Type
}

// PruneStale removes entries not seen since the given time.
// This is a simple garbage collection mechanism.
func (m *DeviceMapper) PruneStale(_ time.Time) {
	// For now this is a no-op — leases manage their own lifecycle
	// and Remove() is called on release/expire. This placeholder
	// exists for future TTL-based cleanup if needed.
}
