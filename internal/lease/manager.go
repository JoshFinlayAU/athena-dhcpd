package lease

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Manager handles lease allocation, renewal, release, and expiry.
type Manager struct {
	store  *Store
	cfg    *config.Config
	bus    *events.Bus
	logger *slog.Logger
	mu     sync.Mutex
}

// NewManager creates a new lease manager.
func NewManager(store *Store, cfg *config.Config, bus *events.Bus, logger *slog.Logger) *Manager {
	return &Manager{
		store:  store,
		cfg:    cfg,
		bus:    bus,
		logger: logger,
	}
}

// Store returns the underlying lease store.
func (m *Manager) Store() *Store {
	return m.store
}

// FindExistingLease looks up an existing lease for a client by client-id, then MAC.
// RFC 2131 §4.2 — client identification priority.
func (m *Manager) FindExistingLease(clientID string, mac net.HardwareAddr) *Lease {
	if clientID != "" {
		if l := m.store.GetByClientID(clientID); l != nil {
			return l
		}
	}
	return m.store.GetByMAC(mac)
}

// FindReservation finds a static reservation for the given MAC or client-id in the config.
func (m *Manager) FindReservation(clientID string, mac net.HardwareAddr, subnetIdx int) *config.ReservationConfig {
	if subnetIdx < 0 || subnetIdx >= len(m.cfg.Subnets) {
		return nil
	}
	sub := m.cfg.Subnets[subnetIdx]
	macStr := mac.String()

	for i, res := range sub.Reservations {
		if res.MAC != "" && res.MAC == macStr {
			return &sub.Reservations[i]
		}
		if res.Identifier != "" && clientID != "" && res.Identifier == clientID {
			return &sub.Reservations[i]
		}
	}
	return nil
}

// CreateOffer creates a new lease in the "offered" state.
func (m *Manager) CreateOffer(ip net.IP, mac net.HardwareAddr, clientID, hostname, subnet, pool string,
	leaseTime time.Duration, relayInfo *RelayInfo) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	l := &Lease{
		IP:          ip,
		MAC:         mac,
		ClientID:    clientID,
		Hostname:    hostname,
		Subnet:      subnet,
		Pool:        pool,
		State:       dhcpv4.LeaseStateOffered,
		Start:       now,
		Expiry:      now.Add(leaseTime),
		LastUpdated: now,
		UpdateSeq:   m.store.NextSeq(),
		RelayInfo:   relayInfo,
	}

	if err := m.store.Put(l); err != nil {
		return nil, fmt.Errorf("creating offer for %s: %w", ip, err)
	}

	metrics.LeaseOperations.WithLabelValues("offer").Inc()
	metrics.LeasesOffered.Inc()

	m.logger.Debug("lease offered",
		"ip", ip.String(),
		"mac", mac.String(),
		"subnet", subnet,
		"msg_type", "DHCPOFFER")

	m.bus.Publish(events.Event{
		Type:      events.EventLeaseOffer,
		Timestamp: now,
		Lease:     m.leaseToEventData(l),
	})

	return l, nil
}

// ConfirmLease transitions a lease from offered to active (DHCPACK).
func (m *Manager) ConfirmLease(ip net.IP, mac net.HardwareAddr, clientID, hostname, subnet, pool string,
	leaseTime time.Duration, relayInfo *RelayInfo) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	existing := m.store.GetByIP(ip)

	isRenew := existing != nil && existing.State == dhcpv4.LeaseStateActive

	l := &Lease{
		IP:          ip,
		MAC:         mac,
		ClientID:    clientID,
		Hostname:    hostname,
		Subnet:      subnet,
		Pool:        pool,
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(leaseTime),
		LastUpdated: now,
		UpdateSeq:   m.store.NextSeq(),
		RelayInfo:   relayInfo,
	}

	if err := m.store.Put(l); err != nil {
		return nil, fmt.Errorf("confirming lease for %s: %w", ip, err)
	}

	metrics.LeasesActive.Inc()
	metrics.LeasesOffered.Dec()

	eventType := events.EventLeaseAck
	if isRenew {
		metrics.LeaseOperations.WithLabelValues("renew").Inc()
		eventType = events.EventLeaseRenew
	} else {
		metrics.LeaseOperations.WithLabelValues("ack").Inc()
	}

	m.logger.Info("lease confirmed",
		"ip", ip.String(),
		"mac", mac.String(),
		"subnet", subnet,
		"is_renew", isRenew,
		"msg_type", "DHCPACK")

	m.bus.Publish(events.Event{
		Type:      eventType,
		Timestamp: now,
		Lease:     m.leaseToEventData(l),
	})

	return l, nil
}

// Release handles a DHCPRELEASE — marks the lease as released and removes it.
func (m *Manager) Release(ip net.IP, mac net.HardwareAddr) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.store.GetByIP(ip)
	if l == nil {
		m.logger.Warn("release for unknown lease",
			"ip", ip.String(),
			"mac", mac.String())
		return nil
	}

	// Verify MAC matches
	if l.MAC.String() != mac.String() {
		m.logger.Warn("release MAC mismatch",
			"ip", ip.String(),
			"lease_mac", l.MAC.String(),
			"release_mac", mac.String())
	}

	eventData := m.leaseToEventData(l)

	if err := m.store.Delete(ip); err != nil {
		return fmt.Errorf("releasing lease for %s: %w", ip, err)
	}

	metrics.LeaseOperations.WithLabelValues("release").Inc()
	metrics.LeasesActive.Dec()

	m.logger.Info("lease released",
		"ip", ip.String(),
		"mac", mac.String(),
		"msg_type", "DHCPRELEASE")

	m.bus.Publish(events.Event{
		Type:      events.EventLeaseRelease,
		Timestamp: time.Now(),
		Lease:     eventData,
	})

	return nil
}

// Decline handles a DHCPDECLINE — marks the IP as declined and removes the lease.
// RFC 2131 §3.1 — client detected IP conflict.
func (m *Manager) Decline(ip net.IP, mac net.HardwareAddr) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.store.GetByIP(ip)
	eventData := &events.LeaseData{
		IP:    ip,
		MAC:   mac,
		State: string(dhcpv4.LeaseStateDeclined),
	}
	if l != nil {
		eventData = m.leaseToEventData(l)
		eventData.State = string(dhcpv4.LeaseStateDeclined)
	}

	if err := m.store.Delete(ip); err != nil {
		return fmt.Errorf("declining lease for %s: %w", ip, err)
	}

	metrics.LeaseOperations.WithLabelValues("decline").Inc()
	metrics.LeasesActive.Dec()

	m.logger.Warn("lease declined (client detected conflict)",
		"ip", ip.String(),
		"mac", mac.String(),
		"msg_type", "DHCPDECLINE")

	m.bus.Publish(events.Event{
		Type:      events.EventLeaseDecline,
		Timestamp: time.Now(),
		Lease:     eventData,
	})

	return nil
}

// ExpireLeases finds and removes expired leases. Called by the GC goroutine.
func (m *Manager) ExpireLeases() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expired []*Lease
	m.store.ForEach(func(l *Lease) bool {
		if l.IsExpired() && l.State != dhcpv4.LeaseStateExpired {
			expired = append(expired, l.Clone())
		}
		return true
	})

	for _, l := range expired {
		eventData := m.leaseToEventData(l)

		if err := m.store.Delete(l.IP); err != nil {
			m.logger.Error("failed to delete expired lease",
				"ip", l.IP.String(),
				"error", err)
			continue
		}

		metrics.LeaseOperations.WithLabelValues("expire").Inc()
		metrics.LeasesActive.Dec()

		m.logger.Info("lease expired",
			"ip", l.IP.String(),
			"mac", l.MAC.String(),
			"subnet", l.Subnet)

		m.bus.Publish(events.Event{
			Type:      events.EventLeaseExpire,
			Timestamp: time.Now(),
			Lease:     eventData,
		})
	}

	return len(expired)
}

// leaseToEventData converts a lease to event payload.
func (m *Manager) leaseToEventData(l *Lease) *events.LeaseData {
	d := &events.LeaseData{
		IP:       l.IP,
		MAC:      l.MAC,
		ClientID: l.ClientID,
		Hostname: l.Hostname,
		FQDN:     l.FQDN,
		Subnet:   l.Subnet,
		Pool:     l.Pool,
		Start:    l.Start.Unix(),
		Expiry:   l.Expiry.Unix(),
		State:    string(l.State),
	}
	if l.RelayInfo != nil {
		d.Relay = &events.RelayData{
			GIAddr:    l.RelayInfo.GIAddr,
			CircuitID: l.RelayInfo.CircuitID,
			RemoteID:  l.RelayInfo.RemoteID,
		}
	}
	return d
}

// GetConfig returns the current config.
func (m *Manager) GetConfig() *config.Config {
	return m.cfg
}

// UpdateConfig updates the config (for hot-reload).
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// StartGC starts the lease garbage collection goroutine.
func (m *Manager) StartGC(ctx context.Context, interval time.Duration) {
	go m.gcLoop(ctx, interval)
}

func (m *Manager) gcLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n := m.ExpireLeases()
			if n > 0 {
				m.logger.Info("lease GC completed", "expired_count", n)
			}
		}
	}
}
