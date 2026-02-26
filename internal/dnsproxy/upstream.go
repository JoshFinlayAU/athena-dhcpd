package dnsproxy

import (
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// UpstreamStats holds latency and reliability stats for an upstream resolver.
type UpstreamStats struct {
	Address     string  `json:"address"`
	AvgLatency  float64 `json:"avg_latency_ms"`
	MinLatency  float64 `json:"min_latency_ms"`
	MaxLatency  float64 `json:"max_latency_ms"`
	LastLatency float64 `json:"last_latency_ms"`
	Successes   int64   `json:"successes"`
	Failures    int64   `json:"failures"`
	Reliability float64 `json:"reliability_pct"`
	LastCheck   string  `json:"last_check"`
	Healthy     bool    `json:"healthy"`
}

// UpstreamTracker measures latency and reliability of upstream DNS servers
// and selects the fastest available one for each query.
type UpstreamTracker struct {
	mu       sync.RWMutex
	servers  map[string]*upstreamState
	order    []string // addresses sorted by latency
	logger   *slog.Logger
	done     chan struct{}
	interval time.Duration

	// EWMA smoothing factor (0-1, higher = more weight on recent samples)
	alpha float64
}

type upstreamState struct {
	address    string
	avgLatency float64 // EWMA in ms
	minLatency float64
	maxLatency float64
	lastLatency float64
	successes  int64
	failures   int64
	lastCheck  time.Time
	healthy    bool
	// consecutive failures for health marking
	consecutiveFail int
}

// NewUpstreamTracker creates a tracker for the given upstream addresses.
func NewUpstreamTracker(addresses []string, logger *slog.Logger) *UpstreamTracker {
	servers := make(map[string]*upstreamState, len(addresses))
	order := make([]string, 0, len(addresses))

	for _, addr := range addresses {
		if !strings.Contains(addr, ":") {
			addr = addr + ":53"
		}
		servers[addr] = &upstreamState{
			address:    addr,
			avgLatency: 50, // initial estimate 50ms
			minLatency: math.MaxFloat64,
			healthy:    true,
		}
		order = append(order, addr)
	}

	return &UpstreamTracker{
		servers:  servers,
		order:    order,
		logger:   logger,
		done:     make(chan struct{}),
		interval: 10 * time.Second,
		alpha:    0.3,
	}
}

// Start begins periodic latency probing of all upstream servers.
func (t *UpstreamTracker) Start() {
	go t.probeLoop()
}

// Stop halts the probe loop.
func (t *UpstreamTracker) Stop() {
	close(t.done)
}

// RecordSuccess records a successful query to an upstream with the given latency.
func (t *UpstreamTracker) RecordSuccess(addr string, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.servers[addr]
	if !ok {
		return
	}

	ms := float64(latency.Microseconds()) / 1000.0
	s.successes++
	s.lastLatency = ms
	s.lastCheck = time.Now()
	s.consecutiveFail = 0
	s.healthy = true

	// EWMA update
	s.avgLatency = t.alpha*ms + (1-t.alpha)*s.avgLatency

	if ms < s.minLatency {
		s.minLatency = ms
	}
	if ms > s.maxLatency {
		s.maxLatency = ms
	}

	t.reorder()
}

// RecordFailure records a failed query attempt to an upstream.
func (t *UpstreamTracker) RecordFailure(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.servers[addr]
	if !ok {
		return
	}

	s.failures++
	s.lastCheck = time.Now()
	s.consecutiveFail++

	// Mark unhealthy after 3 consecutive failures
	if s.consecutiveFail >= 3 {
		s.healthy = false
	}

	t.reorder()
}

// BestServers returns upstream addresses sorted by latency, healthy first.
func (t *UpstreamTracker) BestServers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]string, len(t.order))
	copy(result, t.order)
	return result
}

// Stats returns stats for all tracked upstreams.
func (t *UpstreamTracker) Stats() []UpstreamStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]UpstreamStats, 0, len(t.servers))
	for _, addr := range t.order {
		s := t.servers[addr]
		total := s.successes + s.failures
		reliability := 100.0
		if total > 0 {
			reliability = float64(s.successes) / float64(total) * 100
		}
		minLat := s.minLatency
		if minLat == math.MaxFloat64 {
			minLat = 0
		}
		lastCheck := ""
		if !s.lastCheck.IsZero() {
			lastCheck = s.lastCheck.Format(time.RFC3339)
		}
		result = append(result, UpstreamStats{
			Address:     s.address,
			AvgLatency:  math.Round(s.avgLatency*100) / 100,
			MinLatency:  math.Round(minLat*100) / 100,
			MaxLatency:  math.Round(s.maxLatency*100) / 100,
			LastLatency: math.Round(s.lastLatency*100) / 100,
			Successes:   s.successes,
			Failures:    s.failures,
			Reliability: math.Round(reliability*100) / 100,
			LastCheck:   lastCheck,
			Healthy:     s.healthy,
		})
	}
	return result
}

// reorder sorts the order slice: healthy first, then by avg latency. Caller must hold mu.
func (t *UpstreamTracker) reorder() {
	sort.SliceStable(t.order, func(i, j int) bool {
		si := t.servers[t.order[i]]
		sj := t.servers[t.order[j]]

		// Healthy before unhealthy
		if si.healthy != sj.healthy {
			return si.healthy
		}
		// Then by avg latency
		return si.avgLatency < sj.avgLatency
	})
}

// probeLoop periodically sends test queries to all upstreams.
func (t *UpstreamTracker) probeLoop() {
	// Initial probe immediately
	t.probeAll()

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.probeAll()
		case <-t.done:
			return
		}
	}
}

// probeAll sends a test query to each upstream and records results.
func (t *UpstreamTracker) probeAll() {
	t.mu.RLock()
	addrs := make([]string, len(t.order))
	copy(addrs, t.order)
	t.mu.RUnlock()

	client := &dns.Client{Timeout: 3 * time.Second}

	// Probe with a simple query for "." NS
	msg := new(dns.Msg)
	msg.SetQuestion(".", dns.TypeNS)
	msg.RecursionDesired = true

	for _, addr := range addrs {
		start := time.Now()
		_, _, err := client.Exchange(msg, addr)
		elapsed := time.Since(start)

		if err != nil {
			t.RecordFailure(addr)
			t.logger.Debug("upstream probe failed", "server", addr, "error", err)
		} else {
			t.RecordSuccess(addr, elapsed)
			t.logger.Debug("upstream probe ok", "server", addr, "latency_ms", float64(elapsed.Microseconds())/1000)
		}
	}
}
