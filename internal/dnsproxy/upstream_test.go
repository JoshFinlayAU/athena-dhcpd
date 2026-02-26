package dnsproxy

import (
	"testing"
	"time"
)

func TestUpstreamTrackerBestServers(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}, testLogger())

	// Record varying latencies
	tracker.RecordSuccess("8.8.8.8:53", 10*time.Millisecond)
	tracker.RecordSuccess("1.1.1.1:53", 50*time.Millisecond)
	tracker.RecordSuccess("9.9.9.9:53", 5*time.Millisecond)

	best := tracker.BestServers()
	if len(best) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(best))
	}

	// 9.9.9.9 should be first (fastest), then 8.8.8.8, then 1.1.1.1
	if best[0] != "9.9.9.9:53" {
		t.Errorf("expected 9.9.9.9:53 first, got %s", best[0])
	}
	if best[1] != "8.8.8.8:53" {
		t.Errorf("expected 8.8.8.8:53 second, got %s", best[1])
	}
}

func TestUpstreamTrackerHealthy(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"fast.dns:53", "slow.dns:53"}, testLogger())

	// Fast server has low latency
	tracker.RecordSuccess("fast.dns:53", 5*time.Millisecond)
	// Slow server has 3 consecutive failures -> unhealthy
	tracker.RecordFailure("slow.dns:53")
	tracker.RecordFailure("slow.dns:53")
	tracker.RecordFailure("slow.dns:53")

	best := tracker.BestServers()
	// Healthy server should be first even if slow would otherwise be ordered differently
	if best[0] != "fast.dns:53" {
		t.Errorf("expected healthy server first, got %s", best[0])
	}

	stats := tracker.Stats()
	for _, s := range stats {
		if s.Address == "slow.dns:53" && s.Healthy {
			t.Error("slow.dns should be unhealthy after 3 failures")
		}
		if s.Address == "fast.dns:53" && !s.Healthy {
			t.Error("fast.dns should be healthy")
		}
	}
}

func TestUpstreamTrackerRecovery(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"server:53"}, testLogger())

	// Mark unhealthy
	tracker.RecordFailure("server:53")
	tracker.RecordFailure("server:53")
	tracker.RecordFailure("server:53")

	stats := tracker.Stats()
	if stats[0].Healthy {
		t.Error("should be unhealthy after 3 failures")
	}

	// A success should recover it
	tracker.RecordSuccess("server:53", 10*time.Millisecond)
	stats = tracker.Stats()
	if !stats[0].Healthy {
		t.Error("should be healthy after a success")
	}
}

func TestUpstreamTrackerEWMA(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"server:53"}, testLogger())

	// Record a bunch of 10ms latencies
	for i := 0; i < 20; i++ {
		tracker.RecordSuccess("server:53", 10*time.Millisecond)
	}

	stats := tracker.Stats()
	// EWMA should converge near 10ms (started at 50ms default)
	if stats[0].AvgLatency > 15 {
		t.Errorf("EWMA should converge near 10ms after 20 samples, got %.2f", stats[0].AvgLatency)
	}
	if stats[0].MinLatency != 10.0 {
		t.Errorf("min latency should be 10ms, got %.2f", stats[0].MinLatency)
	}
}

func TestUpstreamTrackerReliability(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"server:53"}, testLogger())

	tracker.RecordSuccess("server:53", 10*time.Millisecond)
	tracker.RecordSuccess("server:53", 10*time.Millisecond)
	tracker.RecordFailure("server:53")
	tracker.RecordSuccess("server:53", 10*time.Millisecond)

	stats := tracker.Stats()
	// 3 successes, 1 failure = 75%
	if stats[0].Reliability != 75.0 {
		t.Errorf("reliability should be 75%%, got %.2f%%", stats[0].Reliability)
	}
}

func TestUpstreamTrackerPortAppend(t *testing.T) {
	tracker := NewUpstreamTracker([]string{"1.1.1.1", "8.8.8.8:5353"}, testLogger())

	best := tracker.BestServers()
	found1111 := false
	found8888 := false
	for _, s := range best {
		if s == "1.1.1.1:53" {
			found1111 = true
		}
		if s == "8.8.8.8:5353" {
			found8888 = true
		}
	}
	if !found1111 {
		t.Error("1.1.1.1 should have :53 appended")
	}
	if !found8888 {
		t.Error("8.8.8.8:5353 should keep its port")
	}
}
