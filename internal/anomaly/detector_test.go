package anomaly

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// keep net import for conflict test
var _ = net.ParseIP

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestWeatherTracksSubnets(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := DefaultConfig()
	d := NewDetector(bus, cfg, testLogger())

	// Simulate events directly
	d.handleEvent(events.Event{
		Type: events.EventLeaseDiscover,
		Lease: &events.LeaseData{
			Subnet: "10.0.0.0/24",
			MAC:    "aa:bb:cc:00:00:01",
		},
	})
	d.handleEvent(events.Event{
		Type: events.EventLeaseAck,
		Lease: &events.LeaseData{
			Subnet: "10.0.0.0/24",
			MAC:    "aa:bb:cc:00:00:01",
		},
	})
	d.handleEvent(events.Event{
		Type: events.EventLeaseDiscover,
		Lease: &events.LeaseData{
			Subnet: "192.168.1.0/24",
			MAC:    "aa:bb:cc:00:00:02",
		},
	})

	weather := d.Weather()
	if len(weather) != 2 {
		t.Fatalf("expected 2 subnets, got %d", len(weather))
	}
}

func TestWindowProcessing(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := DefaultConfig()
	d := NewDetector(bus, cfg, testLogger())

	// Add some events
	for i := 0; i < 10; i++ {
		d.handleEvent(events.Event{
			Type: events.EventLeaseDiscover,
			Lease: &events.LeaseData{
				Subnet: "10.0.0.0/24",
				MAC:    fmt.Sprintf("aa:bb:cc:00:00:%02x", i),
			},
		})
	}

	// Process window
	d.processWindow()

	weather := d.Weather()
	if len(weather) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(weather))
	}
	w := weather[0]
	if w.BaselineRate == 0 {
		t.Error("baseline rate should be non-zero after processing")
	}
	if w.KnownMACs != 10 {
		t.Errorf("known MACs = %d, want 10", w.KnownMACs)
	}
}

func TestKnownVsUnknownMACs(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := DefaultConfig()
	d := NewDetector(bus, cfg, testLogger())

	mac1 := "aa:bb:cc:00:00:01"
	mac2 := "aa:bb:cc:00:00:02"

	// First time seeing mac1
	d.handleEvent(events.Event{
		Type:  events.EventLeaseDiscover,
		Lease: &events.LeaseData{Subnet: "10.0.0.0/24", MAC: mac1},
	})
	// mac1 again — should not count as unknown
	d.handleEvent(events.Event{
		Type:  events.EventLeaseDiscover,
		Lease: &events.LeaseData{Subnet: "10.0.0.0/24", MAC: mac1},
	})
	// New mac2
	d.handleEvent(events.Event{
		Type:  events.EventLeaseDiscover,
		Lease: &events.LeaseData{Subnet: "10.0.0.0/24", MAC: mac2},
	})

	d.mu.RLock()
	s := d.subnets["10.0.0.0/24"]
	known := len(s.knownMACs)
	unknown := s.unknownCount
	d.mu.RUnlock()

	if known != 2 {
		t.Errorf("known MACs = %d, want 2", known)
	}
	// Both MACs were new when first seen, so unknownCount = 2
	if unknown != 2 {
		t.Errorf("unknown MACs = %d, want 2", unknown)
	}
}

func TestNonLeaseEventsIgnored(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := DefaultConfig()
	d := NewDetector(bus, cfg, testLogger())

	d.handleEvent(events.Event{
		Type: events.EventConflictDetected,
		Conflict: &events.ConflictData{
			IP:     net.ParseIP("10.0.0.1"),
			Subnet: "10.0.0.0/24",
		},
	})

	weather := d.Weather()
	if len(weather) != 0 {
		t.Error("non-lease events should be ignored")
	}
}

func TestStatusNormal(t *testing.T) {
	bus := events.NewBus(100, testLogger())
	go bus.Start()
	defer bus.Stop()

	cfg := DefaultConfig()
	d := NewDetector(bus, cfg, testLogger())

	// Add moderate traffic and process a few windows
	for w := 0; w < 5; w++ {
		for i := 0; i < 5; i++ {
			d.handleEvent(events.Event{
				Type:  events.EventLeaseDiscover,
				Lease: &events.LeaseData{Subnet: "10.0.0.0/24", MAC: fmt.Sprintf("aa:00:00:00:%02x:%02x", w, i)},
			})
		}
		d.processWindow()
	}

	weather := d.Weather()
	if len(weather) != 1 {
		t.Fatalf("expected 1 subnet")
	}
	if weather[0].Status != "normal" {
		t.Errorf("status = %q, expected normal for steady traffic", weather[0].Status)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WindowSize != time.Minute {
		t.Errorf("WindowSize = %v", cfg.WindowSize)
	}
	if cfg.AlertThreshold != 3.0 {
		t.Errorf("AlertThreshold = %v", cfg.AlertThreshold)
	}
}
