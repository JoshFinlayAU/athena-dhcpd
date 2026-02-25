package events

import (
	"log/slog"
	"sync"

	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// Bus is a non-blocking event bus that fans out events to subscribers.
// The event channel is buffered — if full, events are dropped with a warning.
type Bus struct {
	ch          chan Event
	subscribers []chan Event
	mu          sync.RWMutex
	logger      *slog.Logger
	bufferSize  int
	drops       uint64
	dropsMu     sync.Mutex
	done        chan struct{}
}

// NewBus creates a new event bus with the given buffer size.
func NewBus(bufferSize int, logger *slog.Logger) *Bus {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	return &Bus{
		ch:         make(chan Event, bufferSize),
		logger:     logger,
		bufferSize: bufferSize,
		done:       make(chan struct{}),
	}
}

// Start begins dispatching events to subscribers. Call in a goroutine.
func (b *Bus) Start() {
	for {
		select {
		case evt, ok := <-b.ch:
			if !ok {
				return
			}
			b.mu.RLock()
			for _, sub := range b.subscribers {
				select {
				case sub <- evt:
				default:
					// Subscriber channel full, drop event
					b.logger.Warn("subscriber event buffer full, dropping event",
						"event_type", string(evt.Type))
				}
			}
			b.mu.RUnlock()
		case <-b.done:
			return
		}
	}
}

// Stop shuts down the event bus.
func (b *Bus) Stop() {
	close(b.done)
	close(b.ch)
}

// Publish sends an event to the bus. Non-blocking — drops if buffer is full.
func (b *Bus) Publish(evt Event) {
	metrics.EventsPublished.WithLabelValues(string(evt.Type)).Inc()
	select {
	case b.ch <- evt:
	default:
		b.dropsMu.Lock()
		b.drops++
		b.dropsMu.Unlock()
		metrics.EventBufferDrops.Inc()
		b.logger.Warn("event bus buffer full, dropping event",
			"event_type", string(evt.Type),
			"total_drops", b.drops)
	}
}

// Subscribe returns a new channel that receives all events from the bus.
// The caller should read from the channel to avoid blocking.
func (b *Bus) Subscribe(bufferSize int) chan Event {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	ch := make(chan Event, bufferSize)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel from the bus.
func (b *Bus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subscribers {
		if sub == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// Drops returns the total number of dropped events.
func (b *Bus) Drops() uint64 {
	b.dropsMu.Lock()
	defer b.dropsMu.Unlock()
	return b.drops
}
