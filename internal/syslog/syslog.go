// Package syslog provides a remote syslog forwarder for athena-dhcpd events.
// It subscribes to the event bus and forwards events as RFC 5424 syslog messages
// to a configurable remote syslog server over UDP or TCP.
package syslog

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// Facility values (RFC 5424)
const (
	FacilityDaemon = 3
	FacilityLocal0 = 16
	FacilityLocal7 = 23
)

// Severity values (RFC 5424)
const (
	SeverityEmergency = iota
	SeverityAlert
	SeverityCritical
	SeverityError
	SeverityWarning
	SeverityNotice
	SeverityInfo
	SeverityDebug
)

// Config holds syslog forwarder settings.
type Config struct {
	Address  string // host:port
	Protocol string // "udp" or "tcp"
	Facility int    // syslog facility (default: local0 = 16)
	Tag      string // syslog tag (default: "athena-dhcpd")
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Protocol: "udp",
		Facility: FacilityLocal0,
		Tag:      "athena-dhcpd",
	}
}

// Forwarder subscribes to the event bus and forwards events to a remote syslog server.
type Forwarder struct {
	cfg    Config
	bus    *events.Bus
	logger *slog.Logger
	ch     chan events.Event
	done   chan struct{}

	mu   sync.Mutex
	conn net.Conn
}

// NewForwarder creates a new syslog forwarder.
func NewForwarder(cfg Config, bus *events.Bus, logger *slog.Logger) *Forwarder {
	if cfg.Tag == "" {
		cfg.Tag = "athena-dhcpd"
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "udp"
	}
	if cfg.Facility == 0 {
		cfg.Facility = FacilityLocal0
	}
	return &Forwarder{
		cfg:    cfg,
		bus:    bus,
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Start subscribes to the event bus and begins forwarding.
func (f *Forwarder) Start() error {
	if f.cfg.Address == "" {
		return fmt.Errorf("syslog address not configured")
	}

	conn, err := net.DialTimeout(f.cfg.Protocol, f.cfg.Address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to syslog %s://%s: %w", f.cfg.Protocol, f.cfg.Address, err)
	}

	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()

	f.ch = f.bus.Subscribe(500)

	f.logger.Info("syslog forwarder started",
		"address", f.cfg.Address,
		"protocol", f.cfg.Protocol,
		"facility", f.cfg.Facility)

	go f.loop()
	return nil
}

// Stop shuts down the forwarder.
func (f *Forwarder) Stop() {
	close(f.done)
	if f.ch != nil {
		f.bus.Unsubscribe(f.ch)
	}
	f.mu.Lock()
	if f.conn != nil {
		f.conn.Close()
	}
	f.mu.Unlock()
	f.logger.Info("syslog forwarder stopped")
}

func (f *Forwarder) loop() {
	for {
		select {
		case evt, ok := <-f.ch:
			if !ok {
				return
			}
			f.forward(evt)
		case <-f.done:
			return
		}
	}
}

func (f *Forwarder) forward(evt events.Event) {
	severity := eventSeverity(evt.Type)
	priority := f.cfg.Facility*8 + severity

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "-"
	}

	msg := formatEvent(evt)

	// RFC 5424 format: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID MSG
	ts := evt.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
	line := fmt.Sprintf("<%d>1 %s %s %s - - - %s\n",
		priority, ts, hostname, f.cfg.Tag, msg)

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.conn == nil {
		return
	}

	if _, err := f.conn.Write([]byte(line)); err != nil {
		f.logger.Debug("syslog write failed, reconnecting", "error", err)
		f.conn.Close()
		// Try reconnect
		conn, err := net.DialTimeout(f.cfg.Protocol, f.cfg.Address, 3*time.Second)
		if err != nil {
			f.logger.Warn("syslog reconnect failed", "error", err)
			f.conn = nil
			return
		}
		f.conn = conn
		f.conn.Write([]byte(line))
	}
}

// FormatMessage formats an event into a syslog message string (exported for testing).
func FormatMessage(evt events.Event) string {
	return formatEvent(evt)
}

func formatEvent(evt events.Event) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("event=%s", evt.Type))

	if evt.Lease != nil {
		l := evt.Lease
		if l.IP != nil {
			parts = append(parts, fmt.Sprintf("ip=%s", l.IP))
		}
		if l.MAC != nil {
			parts = append(parts, fmt.Sprintf("mac=%s", l.MAC))
		}
		if l.Hostname != "" {
			parts = append(parts, fmt.Sprintf("hostname=%s", l.Hostname))
		}
		if l.Subnet != "" {
			parts = append(parts, fmt.Sprintf("subnet=%s", l.Subnet))
		}
		if l.ClientID != "" {
			parts = append(parts, fmt.Sprintf("client_id=%s", l.ClientID))
		}
	}

	if evt.Conflict != nil {
		c := evt.Conflict
		if c.IP != nil {
			parts = append(parts, fmt.Sprintf("conflict_ip=%s", c.IP))
		}
		parts = append(parts, fmt.Sprintf("method=%s", c.DetectionMethod))
	}

	if evt.Rogue != nil {
		r := evt.Rogue
		if r.ServerIP != nil {
			parts = append(parts, fmt.Sprintf("rogue_server=%s", r.ServerIP))
		}
		parts = append(parts, fmt.Sprintf("count=%d", r.Count))
	}

	if evt.Reason != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", evt.Reason))
	}

	return strings.Join(parts, " ")
}

func eventSeverity(t events.EventType) int {
	switch t {
	case events.EventRogueDetected:
		return SeverityWarning
	case events.EventConflictDetected, events.EventConflictPermanent:
		return SeverityWarning
	case events.EventAnomalyDetected:
		return SeverityWarning
	case events.EventLeaseDecline:
		return SeverityNotice
	case events.EventHAFailover:
		return SeverityNotice
	default:
		return SeverityInfo
	}
}
