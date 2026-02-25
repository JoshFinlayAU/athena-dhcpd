package conflict

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Detector coordinates IP conflict detection using ARP and ICMP probes.
// It auto-selects ARP vs ICMP based on whether the candidate IP is on a local subnet.
type Detector struct {
	arp          *ARPProber
	icmp         *ICMPProber
	table        *Table
	cache        *ProbeCache
	bus          *events.Bus
	logger       *slog.Logger
	probeTimeout time.Duration
	maxProbes    int
	strategy     string // "sequential" or "parallel"
	parallelN    int
	localNets    []*net.IPNet // Directly-attached subnets
	gratuitous   bool
}

// DetectorConfig holds configuration for the conflict detector.
type DetectorConfig struct {
	ProbeTimeout     time.Duration
	MaxProbes        int
	Strategy         string
	ParallelCount    int
	HoldTime         time.Duration
	MaxConflictCount int
	CacheTTL         time.Duration
	SendGratuitous   bool
	ICMPFallback     bool
}

// NewDetector creates a new conflict detector.
func NewDetector(
	arp *ARPProber,
	icmpProber *ICMPProber,
	table *Table,
	bus *events.Bus,
	logger *slog.Logger,
	cfg DetectorConfig,
) *Detector {
	cache := NewProbeCache(cfg.CacheTTL)

	d := &Detector{
		arp:          arp,
		icmp:         icmpProber,
		table:        table,
		cache:        cache,
		bus:          bus,
		logger:       logger,
		probeTimeout: cfg.ProbeTimeout,
		maxProbes:    cfg.MaxProbes,
		strategy:     cfg.Strategy,
		parallelN:    cfg.ParallelCount,
		gratuitous:   cfg.SendGratuitous,
	}

	// Discover local subnets from ARP prober's interface
	if arp != nil && arp.Available() {
		iface := arp.Interface()
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
					d.localNets = append(d.localNets, ipNet)
				}
			}
		}
	}

	return d
}

// ProbeResult represents the outcome of a conflict probe.
type ProbeResult struct {
	IP           net.IP
	Conflict     bool
	Method       string // "arp_probe" or "icmp_probe"
	ResponderMAC string
	Duration     time.Duration
	Err          error
	CacheHit     bool
}

// isLocal returns true if the IP is on a directly-attached subnet (use ARP).
func (d *Detector) isLocal(ip net.IP) bool {
	for _, n := range d.localNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ProbeIP probes a single IP for conflicts. Called before DHCPOFFER.
// RFC 2131 §4.4.1 — probe candidate IP before OFFER.
func (d *Detector) ProbeIP(ctx context.Context, ip net.IP, subnet string) ProbeResult {
	start := time.Now()

	// Check conflict table first
	if d.table.IsConflicted(ip) {
		return ProbeResult{
			IP:       ip,
			Conflict: true,
			Method:   "conflict_table",
			Duration: time.Since(start),
		}
	}

	// Check probe cache
	if d.cache.IsClear(ip) {
		metrics.ProbeCacheHits.Inc()
		return ProbeResult{
			IP:       ip,
			Conflict: false,
			CacheHit: true,
			Duration: time.Since(start),
		}
	}
	metrics.ProbeCacheMisses.Inc()

	// Create probe context with timeout
	probeCtx, cancel := context.WithTimeout(ctx, d.probeTimeout)
	defer cancel()

	var conflict bool
	var responderMAC string
	var method string
	var err error

	if d.isLocal(ip) && d.arp != nil && d.arp.Available() {
		// ARP probe for local subnets
		method = string(dhcpv4.DetectionARPProbe)
		conflict, responderMAC, err = d.arp.Probe(probeCtx, ip)
	} else if d.icmp != nil && d.icmp.Available() {
		// ICMP probe for remote/relayed subnets
		method = string(dhcpv4.DetectionICMPProbe)
		conflict, err = d.icmp.Probe(probeCtx, ip)
	} else {
		// No prober available — log warning, assume clear
		d.logger.Warn("no probe method available for IP, assuming clear",
			"ip", ip.String(),
			"subnet", subnet)
		return ProbeResult{
			IP:       ip,
			Conflict: false,
			Duration: time.Since(start),
		}
	}

	duration := time.Since(start)

	if err != nil {
		metrics.ConflictProbes.WithLabelValues(method, "error").Inc()
		metrics.ConflictProbeDuration.WithLabelValues(method).Observe(duration.Seconds())
		d.logger.Error("probe error",
			"ip", ip.String(),
			"method", method,
			"error", err,
			"duration", duration.String())
		return ProbeResult{IP: ip, Err: err, Duration: duration}
	}

	metrics.ConflictProbeDuration.WithLabelValues(method).Observe(duration.Seconds())

	if conflict {
		metrics.ConflictProbes.WithLabelValues(method, "conflict").Inc()
		metrics.ConflictsActive.WithLabelValues(subnet).Inc()
		d.logger.Warn("IP conflict detected",
			"ip", ip.String(),
			"method", method,
			"responder_mac", responderMAC,
			"subnet", subnet,
			"duration", duration.String())

		// Add to conflict table
		permanent, tableErr := d.table.Add(ip, method, responderMAC, subnet)
		if tableErr != nil {
			d.logger.Error("failed to record conflict",
				"ip", ip.String(),
				"error", tableErr)
		}

		// Update cache
		d.cache.MarkConflict(ip)

		// Fire conflict event
		eventType := events.EventConflictDetected
		if permanent {
			eventType = events.EventConflictPermanent
		}

		d.bus.Publish(events.Event{
			Type:      eventType,
			Timestamp: time.Now(),
			Conflict: &events.ConflictData{
				IP:              ip,
				Subnet:          subnet,
				DetectionMethod: method,
				ResponderMAC:    responderMAC,
				ProbeCount:      d.table.Get(ip).ProbeCount,
			},
		})
	} else {
		metrics.ConflictProbes.WithLabelValues(method, "clear").Inc()
		d.cache.MarkClear(ip)
		d.logger.Debug("IP clear after probe",
			"ip", ip.String(),
			"method", method,
			"duration", duration.String())
	}

	return ProbeResult{
		IP:           ip,
		Conflict:     conflict,
		Method:       method,
		ResponderMAC: responderMAC,
		Duration:     duration,
	}
}

// ProbeAndSelect probes candidate IPs and returns the first clear one.
// Uses the configured strategy (sequential or parallel).
func (d *Detector) ProbeAndSelect(ctx context.Context, candidates []net.IP, subnet string) (net.IP, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidate IPs to probe")
	}

	if d.strategy == "parallel" {
		return d.probeParallel(ctx, candidates, subnet)
	}
	return d.probeSequential(ctx, candidates, subnet)
}

// probeSequential probes IPs one at a time, returning the first clear one.
func (d *Detector) probeSequential(ctx context.Context, candidates []net.IP, subnet string) (net.IP, error) {
	maxProbes := d.maxProbes
	if maxProbes > len(candidates) {
		maxProbes = len(candidates)
	}

	for i := 0; i < maxProbes; i++ {
		result := d.ProbeIP(ctx, candidates[i], subnet)
		if result.Err != nil {
			// On error, skip this IP and try next
			continue
		}
		if !result.Conflict {
			return candidates[i], nil
		}
	}

	// All probed IPs conflicted — fall back to offering without probe (with warning)
	if len(candidates) > maxProbes {
		d.logger.Warn("all probed IPs conflicted, offering without probe",
			"probed_count", maxProbes,
			"subnet", subnet)
		return candidates[maxProbes], nil
	}

	return nil, fmt.Errorf("all %d candidate IPs had conflicts in subnet %s", maxProbes, subnet)
}

// probeParallel probes multiple IPs simultaneously, returning the first clear one.
// Caps worst-case latency at one probe_timeout regardless of conflict count.
func (d *Detector) probeParallel(ctx context.Context, candidates []net.IP, subnet string) (net.IP, error) {
	n := d.parallelN
	if n > len(candidates) {
		n = len(candidates)
	}

	type resultMsg struct {
		idx    int
		result ProbeResult
	}

	results := make(chan resultMsg, n)

	probeCtx, cancel := context.WithTimeout(ctx, d.probeTimeout)
	defer cancel()

	for i := 0; i < n; i++ {
		go func(idx int) {
			r := d.ProbeIP(probeCtx, candidates[idx], subnet)
			results <- resultMsg{idx: idx, result: r}
		}(i)
	}

	// Collect results, return first clear IP
	var firstClear net.IP
	received := 0
	for received < n {
		select {
		case msg := <-results:
			received++
			if !msg.result.Conflict && msg.result.Err == nil && firstClear == nil {
				firstClear = candidates[msg.idx]
				cancel() // Stop other probes
			}
		case <-ctx.Done():
			if firstClear != nil {
				return firstClear, nil
			}
			return nil, ctx.Err()
		}
	}

	if firstClear != nil {
		return firstClear, nil
	}

	return nil, fmt.Errorf("all %d parallel-probed IPs had conflicts in subnet %s", n, subnet)
}

// HandleDecline processes a DHCPDECLINE — marks IP as conflicted.
// RFC 2131 §3.1 — client detected IP conflict.
func (d *Detector) HandleDecline(ip net.IP, clientMAC net.HardwareAddr, subnet string) {
	d.logger.Warn("DHCPDECLINE received — client detected conflict",
		"ip", ip.String(),
		"client_mac", clientMAC.String(),
		"subnet", subnet)

	// Add to conflict table with client_decline method
	permanent, err := d.table.Add(ip, string(dhcpv4.DetectionClientDecline), clientMAC.String(), subnet)
	if err != nil {
		d.logger.Error("failed to record decline conflict",
			"ip", ip.String(),
			"error", err)
	}

	// Invalidate probe cache
	d.cache.Invalidate(ip)

	// Fire event
	eventType := events.EventConflictDecline
	if permanent {
		eventType = events.EventConflictPermanent
	}

	d.bus.Publish(events.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Conflict: &events.ConflictData{
			IP:                ip,
			Subnet:            subnet,
			DetectionMethod:   string(dhcpv4.DetectionClientDecline),
			IntendedClientMAC: clientMAC.String(),
		},
	})
}

// SendGratuitousARPForLease sends a gratuitous ARP after DHCPACK (local subnets only).
func (d *Detector) SendGratuitousARPForLease(clientMAC net.HardwareAddr, assignedIP net.IP) {
	if !d.gratuitous {
		return
	}
	if !d.isLocal(assignedIP) {
		return
	}
	SendGratuitousARP(d.arp, clientMAC, assignedIP, d.logger)
}

// Table returns the conflict table.
func (d *Detector) Table() *Table {
	return d.table
}

// Cache returns the probe cache.
func (d *Detector) Cache() *ProbeCache {
	return d.cache
}
