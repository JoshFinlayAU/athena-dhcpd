// Package rogue provides passive detection of rogue DHCP servers on the network.
// It listens for DHCP OFFER and ACK packets from servers other than our own
// and raises alerts via the event bus.
package rogue

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	bolt "go.etcd.io/bbolt"
)

var bucketRogue = []byte("rogue_servers")

// ServerEntry represents a detected rogue DHCP server.
type ServerEntry struct {
	ServerIP    string    `json:"server_ip"`
	ServerMAC   string    `json:"server_mac,omitempty"`
	LastOffer   string    `json:"last_offer_ip,omitempty"`
	LastClient  string    `json:"last_client_mac,omitempty"`
	Interface   string    `json:"interface,omitempty"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Count       int       `json:"count"`
	Acknowledged bool     `json:"acknowledged"`
}

// Detector monitors the network for rogue DHCP servers.
type Detector struct {
	db        *bolt.DB
	bus       *events.Bus
	logger    *slog.Logger
	ownIPs    map[string]bool // our server IPs (to exclude)
	mu        sync.RWMutex
	known     map[string]*ServerEntry // serverIP → entry
	done      chan struct{}
}

// NewDetector creates a new rogue DHCP server detector.
func NewDetector(db *bolt.DB, bus *events.Bus, ownIPs []net.IP, logger *slog.Logger) (*Detector, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketRogue)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("creating rogue bucket: %w", err)
	}

	own := make(map[string]bool, len(ownIPs))
	for _, ip := range ownIPs {
		own[ip.String()] = true
	}

	d := &Detector{
		db:     db,
		bus:    bus,
		logger: logger,
		ownIPs: own,
		known:  make(map[string]*ServerEntry),
		done:   make(chan struct{}),
	}

	// Load persisted rogue server entries
	if err := d.loadAll(); err != nil {
		return nil, fmt.Errorf("loading rogue servers: %w", err)
	}

	return d, nil
}

// ReportOffer is called when our DHCP listener sees an OFFER or ACK from a server.
// If the server IP is not one of ours, it's flagged as rogue.
func (d *Detector) ReportOffer(serverIP, offeredIP net.IP, clientMAC net.HardwareAddr, serverMAC net.HardwareAddr, iface string) {
	sip := serverIP.String()

	// Skip our own server
	if d.ownIPs[sip] {
		return
	}

	d.mu.Lock()

	entry, exists := d.known[sip]
	now := time.Now()

	if exists {
		entry.Count++
		entry.LastSeen = now
		if offeredIP != nil {
			entry.LastOffer = offeredIP.String()
		}
		if clientMAC != nil {
			entry.LastClient = clientMAC.String()
		}
		d.persist(entry)
		d.mu.Unlock()

		d.logger.Warn("rogue DHCP server activity",
			"server_ip", sip,
			"offered_ip", offeredIP,
			"client_mac", clientMAC,
			"count", entry.Count)

		d.bus.Publish(events.Event{
			Type:      events.EventRogueDetected,
			Timestamp: now,
			Rogue: &events.RogueData{
				ServerIP:  serverIP,
				ServerMAC: serverMAC,
				OfferedIP: offeredIP,
				ClientMAC: clientMAC,
				Interface: iface,
				Count:     entry.Count,
			},
		})
		return
	}

	// New rogue server
	entry = &ServerEntry{
		ServerIP:  sip,
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
		Interface: iface,
	}
	if serverMAC != nil {
		entry.ServerMAC = serverMAC.String()
	}
	if offeredIP != nil {
		entry.LastOffer = offeredIP.String()
	}
	if clientMAC != nil {
		entry.LastClient = clientMAC.String()
	}

	d.known[sip] = entry
	d.persist(entry)
	d.mu.Unlock()

	d.logger.Error("NEW rogue DHCP server detected",
		"server_ip", sip,
		"server_mac", serverMAC,
		"offered_ip", offeredIP,
		"client_mac", clientMAC,
		"interface", iface)

	d.bus.Publish(events.Event{
		Type:      events.EventRogueDetected,
		Timestamp: now,
		Rogue: &events.RogueData{
			ServerIP:  serverIP,
			ServerMAC: serverMAC,
			OfferedIP: offeredIP,
			ClientMAC: clientMAC,
			Interface: iface,
			Count:     1,
		},
	})
}

// Acknowledge marks a rogue server as acknowledged (suppresses repeated alerts in UI).
func (d *Detector) Acknowledge(serverIP string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.known[serverIP]
	if !ok {
		return fmt.Errorf("rogue server %s not found", serverIP)
	}
	entry.Acknowledged = true
	d.persist(entry)
	return nil
}

// Remove removes a rogue server entry (e.g. when it's been resolved).
func (d *Detector) Remove(serverIP string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.known[serverIP]; !ok {
		return fmt.Errorf("rogue server %s not found", serverIP)
	}

	delete(d.known, serverIP)

	err := d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRogue)
		return b.Delete([]byte(serverIP))
	})
	if err != nil {
		return fmt.Errorf("deleting rogue server %s: %w", serverIP, err)
	}

	d.bus.Publish(events.Event{
		Type:      events.EventRogueResolved,
		Timestamp: time.Now(),
		Rogue: &events.RogueData{
			ServerIP: net.ParseIP(serverIP),
		},
	})

	return nil
}

// All returns all known rogue servers.
func (d *Detector) All() []ServerEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]ServerEntry, 0, len(d.known))
	for _, e := range d.known {
		result = append(result, *e)
	}
	return result
}

// Count returns the number of known rogue servers.
func (d *Detector) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.known)
}

// ActiveCount returns the number of unacknowledged rogue servers.
func (d *Detector) ActiveCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	count := 0
	for _, e := range d.known {
		if !e.Acknowledged {
			count++
		}
	}
	return count
}

// AddOwnIP adds an IP to the list of our own server IPs.
func (d *Detector) AddOwnIP(ip net.IP) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ownIPs[ip.String()] = true
}

// persist writes a ServerEntry to BoltDB.
func (d *Detector) persist(entry *ServerEntry) {
	data, _ := json.Marshal(entry)
	d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRogue)
		return b.Put([]byte(entry.ServerIP), data)
	})
}

// loadAll loads all rogue server entries from BoltDB.
func (d *Detector) loadAll() error {
	return d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRogue)
		return b.ForEach(func(k, v []byte) error {
			var entry ServerEntry
			if err := json.Unmarshal(v, &entry); err == nil {
				d.known[entry.ServerIP] = &entry
			}
			return nil
		})
	})
}
