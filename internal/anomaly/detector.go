// Package anomaly tracks DHCP activity baselines per subnet and detects
// anomalies: burst of DISCOVERs from unknown MACs, sudden drops to zero,
// unusual request patterns. Fires events for alerting.
package anomaly

import (
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// SubnetWeather holds the current activity state for a subnet.
type SubnetWeather struct {
	Subnet          string  `json:"subnet"`
	CurrentRate     float64 `json:"current_rate"`      // events per minute (last window)
	BaselineRate    float64 `json:"baseline_rate"`      // EWMA of rate
	StdDev          float64 `json:"std_dev"`            // EWMA of standard deviation
	KnownMACs       int     `json:"known_macs"`
	UnknownMACs     int     `json:"unknown_macs_recent"`
	LastActivity    string  `json:"last_activity"`
	SilentMinutes   int     `json:"silent_minutes"`
	AnomalyScore    float64 `json:"anomaly_score"`      // 0 = normal, >2 = notable, >4 = alert
	AnomalyReason   string  `json:"anomaly_reason,omitempty"`
	Status          string  `json:"status"`             // "normal", "elevated", "alert", "silent"
}

// Config holds anomaly detection settings.
type Config struct {
	WindowSize      time.Duration // aggregation window (default 1m)
	BaselineAlpha   float64       // EWMA smoothing for baseline (default 0.1)
	AlertThreshold  float64       // z-score threshold for alerts (default 3.0)
	SilentThreshold int           // minutes of silence before alerting (default 10)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		WindowSize:      1 * time.Minute,
		BaselineAlpha:   0.1,
		AlertThreshold:  3.0,
		SilentThreshold: 10,
	}
}

// Detector monitors DHCP activity and detects anomalies.
type Detector struct {
	bus    *events.Bus
	logger *slog.Logger
	cfg    Config
	ch     chan events.Event
	done   chan struct{}

	mu      sync.RWMutex
	subnets map[string]*subnetState
}

type subnetState struct {
	// Current window counters
	windowStart  time.Time
	windowCount  int
	unknownCount int

	// Known MACs (seen before)
	knownMACs map[string]bool

	// Baseline stats (EWMA)
	baselineRate float64
	baselineVar  float64 // variance for stddev

	// Tracking
	lastActivity time.Time
	lastRate     float64
}

// NewDetector creates a new anomaly detector.
func NewDetector(bus *events.Bus, cfg Config, logger *slog.Logger) *Detector {
	return &Detector{
		bus:     bus,
		logger:  logger,
		cfg:     cfg,
		done:    make(chan struct{}),
		subnets: make(map[string]*subnetState),
	}
}

// Start subscribes to the event bus and begins monitoring.
func (d *Detector) Start() {
	d.ch = d.bus.Subscribe(2000)
	d.logger.Info("anomaly detector started")

	ticker := time.NewTicker(d.cfg.WindowSize)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-d.ch:
			if !ok {
				return
			}
			d.handleEvent(evt)
		case <-ticker.C:
			d.processWindow()
		case <-d.done:
			return
		}
	}
}

// Stop shuts down the anomaly detector.
func (d *Detector) Stop() {
	close(d.done)
	if d.ch != nil {
		d.bus.Unsubscribe(d.ch)
	}
	d.logger.Info("anomaly detector stopped")
}

// Weather returns the current weather for all monitored subnets.
func (d *Detector) Weather() []SubnetWeather {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now()
	result := make([]SubnetWeather, 0, len(d.subnets))
	for subnet, s := range d.subnets {
		stddev := math.Sqrt(s.baselineVar)
		silentMin := 0
		if !s.lastActivity.IsZero() {
			silentMin = int(now.Sub(s.lastActivity).Minutes())
		}

		score, reason, status := d.computeAnomaly(s, silentMin, stddev)

		lastAct := ""
		if !s.lastActivity.IsZero() {
			lastAct = s.lastActivity.Format(time.RFC3339)
		}

		result = append(result, SubnetWeather{
			Subnet:        subnet,
			CurrentRate:   math.Round(s.lastRate*100) / 100,
			BaselineRate:  math.Round(s.baselineRate*100) / 100,
			StdDev:        math.Round(stddev*100) / 100,
			KnownMACs:     len(s.knownMACs),
			UnknownMACs:   s.unknownCount,
			LastActivity:  lastAct,
			SilentMinutes: silentMin,
			AnomalyScore:  math.Round(score*100) / 100,
			AnomalyReason: reason,
			Status:        status,
		})
	}
	return result
}

// handleEvent processes a single event from the bus.
func (d *Detector) handleEvent(evt events.Event) {
	// Only track lease lifecycle events
	switch evt.Type {
	case events.EventLeaseDiscover, events.EventLeaseAck,
		events.EventLeaseRenew, events.EventLeaseRelease,
		events.EventLeaseDecline:
	default:
		return
	}

	if evt.Lease == nil || evt.Lease.Subnet == "" {
		return
	}

	subnet := evt.Lease.Subnet
	mac := macStr(evt.Lease.MAC)

	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.subnets[subnet]
	if !ok {
		s = &subnetState{
			windowStart: time.Now(),
			knownMACs:   make(map[string]bool),
		}
		d.subnets[subnet] = s
	}

	s.windowCount++
	s.lastActivity = time.Now()

	if mac != "" {
		if !s.knownMACs[mac] {
			s.unknownCount++
			s.knownMACs[mac] = true
		}
	}
}

// processWindow runs at each window tick, computing rates and detecting anomalies.
func (d *Detector) processWindow() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	alpha := d.cfg.BaselineAlpha

	for subnet, s := range d.subnets {
		// Calculate rate for this window
		rate := float64(s.windowCount)
		s.lastRate = rate

		// Update EWMA baseline
		if s.baselineRate == 0 && s.windowCount > 0 {
			// First observation
			s.baselineRate = rate
		} else {
			oldBaseline := s.baselineRate
			s.baselineRate = alpha*rate + (1-alpha)*s.baselineRate
			// Update variance using Welford-like EWMA
			diff := rate - oldBaseline
			s.baselineVar = alpha*(diff*diff) + (1-alpha)*s.baselineVar
		}

		// Check for anomalies
		stddev := math.Sqrt(s.baselineVar)
		silentMin := 0
		if !s.lastActivity.IsZero() {
			silentMin = int(now.Sub(s.lastActivity).Minutes())
		}

		score, reason, _ := d.computeAnomaly(s, silentMin, stddev)

		if score >= d.cfg.AlertThreshold {
			d.logger.Warn("anomaly detected",
				"subnet", subnet,
				"score", score,
				"reason", reason,
				"rate", rate,
				"baseline", s.baselineRate)

			d.bus.Publish(events.Event{
				Type:      events.EventAnomalyDetected,
				Timestamp: now,
				Reason:    reason,
				Lease: &events.LeaseData{
					Subnet: subnet,
				},
			})
		}

		// Reset window counters
		s.windowStart = now
		s.windowCount = 0
		s.unknownCount = 0
	}
}

// computeAnomaly calculates anomaly score and status for a subnet.
func (d *Detector) computeAnomaly(s *subnetState, silentMin int, stddev float64) (score float64, reason, status string) {
	status = "normal"

	// Silent subnet detection
	if silentMin >= d.cfg.SilentThreshold && s.baselineRate > 0 {
		score = float64(silentMin) / float64(d.cfg.SilentThreshold)
		reason = "subnet silent"
		status = "silent"
		return
	}

	// Rate spike detection (z-score)
	if stddev > 0 && s.baselineRate > 0 {
		zscore := (s.lastRate - s.baselineRate) / stddev
		if zscore > 2 {
			score = zscore
			reason = "rate spike"
			if zscore >= d.cfg.AlertThreshold {
				status = "alert"
			} else {
				status = "elevated"
			}
			return
		}
		// Sudden drop
		if s.baselineRate > 5 && s.lastRate == 0 {
			score = 2.0
			reason = "sudden drop"
			status = "elevated"
			return
		}
	}

	return 0, "", "normal"
}

func macStr(mac net.HardwareAddr) string {
	if mac == nil {
		return ""
	}
	return mac.String()
}
