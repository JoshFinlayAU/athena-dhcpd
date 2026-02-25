package events

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := NewBus(100, logger)
	go bus.Start()
	defer bus.Stop()

	ch := bus.Subscribe(100)
	defer bus.Unsubscribe(ch)

	evt := Event{
		Type:      EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &LeaseData{
			Hostname: "test",
		},
	}

	bus.Publish(evt)

	select {
	case received := <-ch:
		if received.Type != EventLeaseAck {
			t.Errorf("received event type = %q, want %q", received.Type, EventLeaseAck)
		}
		if received.Lease == nil || received.Lease.Hostname != "test" {
			t.Error("lease data not preserved")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := NewBus(100, logger)
	go bus.Start()
	defer bus.Stop()

	ch1 := bus.Subscribe(100)
	ch2 := bus.Subscribe(100)
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Publish(Event{Type: EventConflictDetected, Timestamp: time.Now()})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Type != EventConflictDetected {
				t.Errorf("event type = %q, want %q", e.Type, EventConflictDetected)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event on subscriber")
		}
	}
}

func TestBusUnsubscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := NewBus(100, logger)
	go bus.Start()
	defer bus.Stop()

	ch := bus.Subscribe(100)
	bus.Unsubscribe(ch)

	// Publish after unsubscribe — should not block or panic
	bus.Publish(Event{Type: EventLeaseRelease, Timestamp: time.Now()})

	// Give a moment for the event to propagate
	time.Sleep(50 * time.Millisecond)

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("should not receive events after unsubscribe")
		}
	default:
		// Expected — channel closed or empty
	}
}

func TestBusNonBlocking(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Tiny buffer
	bus := NewBus(1, logger)
	go bus.Start()
	defer bus.Stop()

	// Publish many events — should not block even with tiny buffer
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			bus.Publish(Event{Type: EventLeaseAck, Timestamp: time.Now()})
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — publishing didn't block
	case <-time.After(2 * time.Second):
		t.Fatal("publishing blocked — event bus should be non-blocking")
	}
}
