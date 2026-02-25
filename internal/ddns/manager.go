package ddns

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// Manager handles dynamic DNS updates asynchronously.
// It subscribes to the event bus and processes lease events to create/remove DNS records.
// DDNS updates are ALWAYS asynchronous to DHCP responses — never block an ACK for DNS.
type Manager struct {
	cfg          *config.DDNSConfig
	forward      DNSUpdater
	reverse      DNSUpdater
	bus          *events.Bus
	logger       *slog.Logger
	ch           chan events.Event
	done         chan struct{}
	wg           sync.WaitGroup
	retryBackoff time.Duration
	maxRetries   int
}

// NewManager creates a new DDNS manager.
func NewManager(cfg *config.DDNSConfig, bus *events.Bus, logger *slog.Logger) (*Manager, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	m := &Manager{
		cfg:          cfg,
		bus:          bus,
		logger:       logger,
		done:         make(chan struct{}),
		retryBackoff: 5 * time.Second,
		maxRetries:   3,
	}

	// Initialize forward zone updater
	fwd, err := m.createUpdater(cfg.Forward)
	if err != nil {
		return nil, fmt.Errorf("creating forward zone updater: %w", err)
	}
	m.forward = fwd

	// Initialize reverse zone updater (optional)
	if cfg.Reverse.Zone != "" {
		rev, err := m.createUpdater(cfg.Reverse)
		if err != nil {
			return nil, fmt.Errorf("creating reverse zone updater: %w", err)
		}
		m.reverse = rev
	}

	return m, nil
}

// createUpdater creates a DNS updater based on the configured method.
func (m *Manager) createUpdater(zoneCfg config.DDNSZoneConfig) (DNSUpdater, error) {
	switch zoneCfg.Method {
	case "rfc2136":
		return NewRFC2136Client(
			zoneCfg.Server,
			zoneCfg.TSIGName,
			zoneCfg.TSIGAlgorithm,
			zoneCfg.TSIGSecret,
			10*time.Second,
			m.logger,
		), nil
	case "powerdns_api":
		return NewPowerDNSClient(
			zoneCfg.Server,
			zoneCfg.APIKey,
			10*time.Second,
			m.logger,
		), nil
	case "technitium_api":
		return NewTechnitiumClient(
			zoneCfg.Server,
			zoneCfg.APIKey,
			10*time.Second,
			m.logger,
		), nil
	default:
		return nil, fmt.Errorf("unsupported DDNS method: %s", zoneCfg.Method)
	}
}

// Start subscribes to the event bus and begins processing DNS updates.
func (m *Manager) Start() {
	m.ch = m.bus.Subscribe(500)

	m.logger.Info("DDNS manager started",
		"forward_zone", m.cfg.Forward.Zone,
		"reverse_zone", m.cfg.Reverse.Zone,
		"method", m.cfg.Forward.Method)

	for {
		select {
		case evt, ok := <-m.ch:
			if !ok {
				return
			}
			m.handleEvent(evt)
		case <-m.done:
			return
		}
	}
}

// Stop shuts down the DDNS manager.
func (m *Manager) Stop() {
	close(m.done)
	if m.ch != nil {
		m.bus.Unsubscribe(m.ch)
	}
	m.wg.Wait()
	m.logger.Info("DDNS manager stopped")
}

// handleEvent processes a single event and dispatches async DNS updates.
func (m *Manager) handleEvent(evt events.Event) {
	switch evt.Type {
	case events.EventLeaseAck:
		if evt.Lease != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.addRecords(evt)
			}()
		}
	case events.EventLeaseRenew:
		if m.cfg.UpdateOnRenew && evt.Lease != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.addRecords(evt)
			}()
		}
	case events.EventLeaseRelease, events.EventLeaseExpire:
		if evt.Lease != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.removeRecords(evt)
			}()
		}
	}
}

// addRecords creates forward (A) and reverse (PTR) DNS records for a lease.
func (m *Manager) addRecords(evt events.Event) {
	l := evt.Lease
	if l.IP == nil || l.MAC == nil {
		return
	}

	// Build FQDN
	fqdn := m.buildFQDN(l)
	if fqdn == "" {
		m.logger.Debug("skipping DDNS update — no FQDN",
			"ip", l.IP.String(), "mac", l.MAC.String())
		return
	}

	zone := m.getForwardZone(l.Subnet)
	ttl := uint32(m.cfg.TTL)

	// Forward A record
	start := time.Now()
	m.withRetry("AddA", fqdn, func() error {
		return m.forward.AddA(zone, fqdn, l.IP, ttl)
	})
	metrics.DDNSUpdates.WithLabelValues("add_a", "success").Inc()
	metrics.DDNSDuration.WithLabelValues("add_a").Observe(time.Since(start).Seconds())

	// Reverse PTR record
	if m.reverse != nil {
		reverseZone := m.getReverseZone(l.Subnet)
		ptrName := ReverseIPName(l.IP)
		ptrStart := time.Now()
		m.withRetry("AddPTR", ptrName, func() error {
			return m.reverse.AddPTR(reverseZone, ptrName, fqdn, ttl)
		})
		metrics.DDNSUpdates.WithLabelValues("add_ptr", "success").Inc()
		metrics.DDNSDuration.WithLabelValues("add_ptr").Observe(time.Since(ptrStart).Seconds())
	}
}

// removeRecords removes forward (A) and reverse (PTR) DNS records for a lease.
func (m *Manager) removeRecords(evt events.Event) {
	l := evt.Lease
	if l.IP == nil || l.MAC == nil {
		return
	}

	fqdn := m.buildFQDN(l)
	if fqdn == "" {
		return
	}

	zone := m.getForwardZone(l.Subnet)

	// Remove forward A record — best-effort
	aStart := time.Now()
	if err := m.forward.RemoveA(zone, fqdn); err != nil {
		metrics.DDNSUpdates.WithLabelValues("remove_a", "error").Inc()
		m.logger.Warn("failed to remove A record (best-effort)",
			"fqdn", fqdn, "error", err)
	} else {
		metrics.DDNSUpdates.WithLabelValues("remove_a", "success").Inc()
	}
	metrics.DDNSDuration.WithLabelValues("remove_a").Observe(time.Since(aStart).Seconds())

	// Remove reverse PTR record — best-effort
	if m.reverse != nil {
		reverseZone := m.getReverseZone(l.Subnet)
		ptrName := ReverseIPName(l.IP)
		ptrStart := time.Now()
		if err := m.reverse.RemovePTR(reverseZone, ptrName); err != nil {
			metrics.DDNSUpdates.WithLabelValues("remove_ptr", "error").Inc()
			m.logger.Warn("failed to remove PTR record (best-effort)",
				"ptr", ptrName, "error", err)
		} else {
			metrics.DDNSUpdates.WithLabelValues("remove_ptr", "success").Inc()
		}
		metrics.DDNSDuration.WithLabelValues("remove_ptr").Observe(time.Since(ptrStart).Seconds())
	}
}

// buildFQDN constructs the FQDN for a lease.
// Priority: client FQDN (option 81) → hostname+domain → MAC fallback → skip.
func (m *Manager) buildFQDN(l *events.LeaseData) string {
	domain := m.cfg.Forward.Zone
	// Strip trailing dot from zone for domain construction
	if len(domain) > 0 && domain[len(domain)-1] == '.' {
		domain = domain[:len(domain)-1]
	}

	var clientFQDN string
	if m.cfg.AllowClientFQDN && l.FQDN != "" {
		clientFQDN = l.FQDN
	}

	hostname := SanitizeHostname(l.Hostname)

	return BuildFQDN(clientFQDN, hostname, domain, l.MAC, m.cfg.FallbackToMAC)
}

// getForwardZone returns the forward zone for a subnet (with override support).
func (m *Manager) getForwardZone(subnet string) string {
	for _, override := range m.cfg.ZoneOverrides {
		if override.Subnet == subnet && override.ForwardZone != "" {
			return override.ForwardZone
		}
	}
	return m.cfg.Forward.Zone
}

// getReverseZone returns the reverse zone for a subnet (with override support).
func (m *Manager) getReverseZone(subnet string) string {
	for _, override := range m.cfg.ZoneOverrides {
		if override.Subnet == subnet && override.ReverseZone != "" {
			return override.ReverseZone
		}
	}
	return m.cfg.Reverse.Zone
}

// withRetry retries an operation with exponential backoff.
func (m *Manager) withRetry(op, name string, fn func() error) {
	var err error
	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(m.retryBackoff * time.Duration(1<<uint(attempt-1)))
		}
		err = fn()
		if err == nil {
			return
		}
		m.logger.Warn("DDNS operation failed, retrying",
			"op", op, "name", name, "attempt", attempt+1,
			"max_retries", m.maxRetries, "error", err)
	}
	m.logger.Error("DDNS operation failed after all retries",
		"op", op, "name", name, "error", err)
}

// UpdateConfig updates the DDNS configuration (for hot-reload).
func (m *Manager) UpdateConfig(cfg *config.DDNSConfig) {
	m.cfg = cfg
}

// ForwardZone returns the configured forward zone name.
func (m *Manager) ForwardZone() string {
	return m.cfg.Forward.Zone
}

// ReverseZone returns the configured reverse zone name.
func (m *Manager) ReverseZone() string {
	return m.cfg.Reverse.Zone
}

// SetForwardUpdater sets the forward DNS updater (for testing).
func (m *Manager) SetForwardUpdater(u DNSUpdater) {
	m.forward = u
}

// SetReverseUpdater sets the reverse DNS updater (for testing).
func (m *Manager) SetReverseUpdater(u DNSUpdater) {
	m.reverse = u
}

// NewManagerForTest creates a manager with mock updaters for testing.
func NewManagerForTest(cfg *config.DDNSConfig, bus *events.Bus, logger *slog.Logger, forward, reverse DNSUpdater) *Manager {
	return &Manager{
		cfg:          cfg,
		forward:      forward,
		reverse:      reverse,
		bus:          bus,
		logger:       logger,
		done:         make(chan struct{}),
		retryBackoff: 10 * time.Millisecond,
		maxRetries:   1,
	}
}

// GetFQDNForLease exposes FQDN construction for testing.
func (m *Manager) GetFQDNForLease(l *events.LeaseData) string {
	return m.buildFQDN(l)
}

// Ensure all clients implement DNSUpdater.
var (
	_ DNSUpdater = (*RFC2136Client)(nil)
	_ DNSUpdater = (*PowerDNSClient)(nil)
	_ DNSUpdater = (*TechnitiumClient)(nil)
)

// reverseIPNameExported is a package-level export for external use.
func init() {
	// Verify ReverseIPName works correctly at init time
	expected := "1.1.168.192.in-addr.arpa"
	got := ReverseIPName(net.IPv4(192, 168, 1, 1))
	if got != expected {
		panic(fmt.Sprintf("ReverseIPName sanity check failed: got %q, want %q", got, expected))
	}
}
