// Package topology builds and maintains a network topology map from DHCP
// Option 82 relay agent data. Circuit-id and remote-id carry physical port
// and switch identity — this package learns those relationships over time
// and builds a tree: switch → port → device.
package topology

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketTopology = []byte("topology")

// SwitchNode represents a relay agent / switch in the topology.
type SwitchNode struct {
	ID        string              `json:"id"`
	RemoteID  string              `json:"remote_id"`
	GIAddr    string              `json:"giaddr"`
	Label     string              `json:"label,omitempty"`
	FirstSeen time.Time           `json:"first_seen"`
	LastSeen  time.Time           `json:"last_seen"`
	Ports     map[string]*PortNode `json:"ports"`
}

// PortNode represents a physical port on a switch.
type PortNode struct {
	CircuitID string        `json:"circuit_id"`
	Label     string        `json:"label,omitempty"`
	FirstSeen time.Time     `json:"first_seen"`
	LastSeen  time.Time     `json:"last_seen"`
	Devices   []*DeviceNode `json:"devices"`
}

// DeviceNode represents a device seen on a port.
type DeviceNode struct {
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname,omitempty"`
	Subnet    string    `json:"subnet,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// LeaseEvent is the input from the DHCP handler when a relay-assisted lease occurs.
type LeaseEvent struct {
	CircuitID string
	RemoteID  string
	GIAddr    string
	MAC       string
	IP        string
	Hostname  string
	Subnet    string
}

// Map holds the full network topology learned from Option 82 data.
type Map struct {
	db      *bolt.DB
	logger  *slog.Logger
	mu      sync.RWMutex
	switches map[string]*SwitchNode // keyed by remote-id or giaddr
}

// NewMap creates a new topology map backed by BoltDB.
func NewMap(db *bolt.DB, logger *slog.Logger) (*Map, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketTopology)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("creating topology bucket: %w", err)
	}

	m := &Map{
		db:       db,
		logger:   logger,
		switches: make(map[string]*SwitchNode),
	}

	if err := m.loadAll(); err != nil {
		return nil, fmt.Errorf("loading topology: %w", err)
	}

	return m, nil
}

// Record processes a lease event and updates the topology map.
func (m *Map) Record(evt LeaseEvent) {
	if evt.CircuitID == "" && evt.RemoteID == "" {
		return // no option 82 data
	}

	now := time.Now()
	// Use remote-id as switch key, fall back to giaddr
	switchKey := evt.RemoteID
	if switchKey == "" {
		switchKey = evt.GIAddr
	}
	if switchKey == "" {
		switchKey = "unknown"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sw, ok := m.switches[switchKey]
	if !ok {
		sw = &SwitchNode{
			ID:        switchKey,
			RemoteID:  evt.RemoteID,
			GIAddr:    evt.GIAddr,
			FirstSeen: now,
			LastSeen:  now,
			Ports:     make(map[string]*PortNode),
		}
		m.switches[switchKey] = sw
	}
	sw.LastSeen = now
	if evt.GIAddr != "" {
		sw.GIAddr = evt.GIAddr
	}

	// Find or create port
	portKey := evt.CircuitID
	if portKey == "" {
		portKey = "unknown"
	}

	port, ok := sw.Ports[portKey]
	if !ok {
		port = &PortNode{
			CircuitID: evt.CircuitID,
			FirstSeen: now,
			LastSeen:  now,
		}
		sw.Ports[portKey] = port
	}
	port.LastSeen = now

	// Update or add device on this port
	found := false
	for _, dev := range port.Devices {
		if dev.MAC == evt.MAC {
			dev.IP = evt.IP
			dev.Hostname = evt.Hostname
			dev.Subnet = evt.Subnet
			dev.LastSeen = now
			found = true
			break
		}
	}
	if !found {
		port.Devices = append(port.Devices, &DeviceNode{
			MAC:       evt.MAC,
			IP:        evt.IP,
			Hostname:  evt.Hostname,
			Subnet:    evt.Subnet,
			FirstSeen: now,
			LastSeen:  now,
		})
	}

	m.persist(switchKey, sw)
}

// SetLabel sets a friendly label for a switch or port.
func (m *Map) SetLabel(switchID, portID, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sw, ok := m.switches[switchID]
	if !ok {
		return fmt.Errorf("switch %q not found", switchID)
	}

	if portID == "" {
		sw.Label = label
	} else {
		port, ok := sw.Ports[portID]
		if !ok {
			return fmt.Errorf("port %q not found on switch %q", portID, switchID)
		}
		port.Label = label
	}

	m.persist(switchID, sw)
	return nil
}

// Tree returns the full topology as a sorted slice of switches.
func (m *Map) Tree() []SwitchNode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SwitchNode, 0, len(m.switches))
	for _, sw := range m.switches {
		cp := *sw
		cp.Ports = make(map[string]*PortNode, len(sw.Ports))
		for k, p := range sw.Ports {
			pCopy := *p
			pCopy.Devices = make([]*DeviceNode, len(p.Devices))
			for i, d := range p.Devices {
				dCopy := *d
				pCopy.Devices[i] = &dCopy
			}
			cp.Ports[k] = &pCopy
		}
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen.After(result[j].LastSeen)
	})
	return result
}

// Stats returns summary statistics about the topology.
func (m *Map) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switches := len(m.switches)
	ports := 0
	devices := 0
	for _, sw := range m.switches {
		ports += len(sw.Ports)
		for _, p := range sw.Ports {
			devices += len(p.Devices)
		}
	}
	return map[string]int{
		"switches": switches,
		"ports":    ports,
		"devices":  devices,
	}
}

// persist writes a switch node to BoltDB.
func (m *Map) persist(key string, sw *SwitchNode) {
	data, _ := json.Marshal(sw)
	m.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTopology)
		return b.Put([]byte(key), data)
	})
}

// loadAll loads topology from BoltDB.
func (m *Map) loadAll() error {
	return m.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTopology)
		return b.ForEach(func(k, v []byte) error {
			var sw SwitchNode
			if err := json.Unmarshal(v, &sw); err == nil {
				m.switches[string(k)] = &sw
			}
			return nil
		})
	})
}
