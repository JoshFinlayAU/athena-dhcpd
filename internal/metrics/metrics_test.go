package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsRegistered(t *testing.T) {
	// Verify key metrics are registered with the default registry.
	// promauto registers automatically, so we just verify they exist
	// by writing a value and collecting it.

	PacketsReceived.WithLabelValues("DHCPDISCOVER").Inc()
	PacketsSent.WithLabelValues("DHCPOFFER").Inc()
	PacketErrors.WithLabelValues("decode").Inc()
	LeaseOperations.WithLabelValues("offer").Inc()
	LeasesActive.Set(42)
	LeasesOffered.Set(3)
	ConflictProbes.WithLabelValues("arp_probe", "clear").Inc()
	EventsPublished.WithLabelValues("lease.ack").Inc()
	EventBufferDrops.Inc()
	ProbeCacheHits.Inc()
	ProbeCacheMisses.Inc()
	HookExecutions.WithLabelValues("script", "success").Inc()
	HAHeartbeatsSent.Inc()
	HAHeartbeatsReceived.Inc()
	HASyncOperations.WithLabelValues("lease_update").Inc()
	HASyncErrors.Inc()
	APIRequests.WithLabelValues("GET", "/api/v1/leases", "200").Inc()
	WebSocketConnections.Set(5)
	DDNSUpdates.WithLabelValues("add_a", "success").Inc()
	PoolSize.WithLabelValues("192.168.1.0/24", "pool1").Set(254)
	PoolAllocated.WithLabelValues("192.168.1.0/24", "pool1").Set(100)
	PoolUtilization.WithLabelValues("192.168.1.0/24", "pool1").Set(39.4)
	PoolExhausted.WithLabelValues("192.168.1.0/24").Inc()
	ServerStartTime.SetToCurrentTime()
	ServerInfo.WithLabelValues("dev").Set(1)

	// Verify a few metrics via testutil
	if got := testutil.ToFloat64(LeasesActive); got != 42 {
		t.Errorf("LeasesActive = %v, want 42", got)
	}
	if got := testutil.ToFloat64(WebSocketConnections); got != 5 {
		t.Errorf("WebSocketConnections = %v, want 5", got)
	}
	if got := testutil.ToFloat64(EventBufferDrops); got != 1 {
		t.Errorf("EventBufferDrops = %v, want 1", got)
	}
	if got := testutil.ToFloat64(ProbeCacheHits); got != 1 {
		t.Errorf("ProbeCacheHits = %v, want 1", got)
	}
}

func TestMetricsNamespace(t *testing.T) {
	// All metrics should use the athena_dhcpd_ namespace
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	for _, mf := range mfs {
		name := mf.GetName()
		// Skip standard go_* and process_* and promhttp_* metrics
		if strings.HasPrefix(name, "go_") ||
			strings.HasPrefix(name, "process_") ||
			strings.HasPrefix(name, "promhttp_") {
			continue
		}
		if !strings.HasPrefix(name, "athena_dhcpd_") {
			t.Errorf("metric %q does not have athena_dhcpd_ prefix", name)
		}
	}
}
