package dnsproxy

import (
	"sync"
	"time"
)

// QueryLogEntry represents a single DNS query and its result.
type QueryLogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Source     string    `json:"source"`
	Status     string    `json:"status"` // "allowed", "blocked", "cached", "local", "forwarded", "failed"
	Latency    float64   `json:"latency_ms"`
	Answer     string    `json:"answer,omitempty"`
	ListName   string    `json:"list_name,omitempty"`
	Action     string    `json:"action,omitempty"`
}

// QueryLog is a thread-safe ring buffer for DNS query log entries.
type QueryLog struct {
	mu       sync.RWMutex
	entries  []QueryLogEntry
	capacity int
	head     int
	count    int

	// Subscribers for live streaming
	subMu   sync.RWMutex
	subs    map[int]chan QueryLogEntry
	nextID  int
}

// NewQueryLog creates a new query log with the given capacity.
func NewQueryLog(capacity int) *QueryLog {
	if capacity <= 0 {
		capacity = 1000
	}
	return &QueryLog{
		entries:  make([]QueryLogEntry, capacity),
		capacity: capacity,
		subs:     make(map[int]chan QueryLogEntry),
	}
}

// Add appends an entry to the log and notifies all subscribers.
func (q *QueryLog) Add(entry QueryLogEntry) {
	q.mu.Lock()
	q.entries[q.head] = entry
	q.head = (q.head + 1) % q.capacity
	if q.count < q.capacity {
		q.count++
	}
	q.mu.Unlock()

	// Notify subscribers (non-blocking)
	q.subMu.RLock()
	for _, ch := range q.subs {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow
		}
	}
	q.subMu.RUnlock()
}

// Recent returns the most recent n entries (newest first).
func (q *QueryLog) Recent(n int) []QueryLogEntry {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if n <= 0 || q.count == 0 {
		return nil
	}
	if n > q.count {
		n = q.count
	}

	result := make([]QueryLogEntry, n)
	for i := 0; i < n; i++ {
		idx := (q.head - 1 - i + q.capacity) % q.capacity
		result[i] = q.entries[idx]
	}
	return result
}

// Subscribe returns a channel that receives new query log entries.
func (q *QueryLog) Subscribe(bufSize int) (int, chan QueryLogEntry) {
	q.subMu.Lock()
	defer q.subMu.Unlock()
	id := q.nextID
	q.nextID++
	ch := make(chan QueryLogEntry, bufSize)
	q.subs[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber.
func (q *QueryLog) Unsubscribe(id int) {
	q.subMu.Lock()
	defer q.subMu.Unlock()
	if ch, ok := q.subs[id]; ok {
		close(ch)
		delete(q.subs, id)
	}
}

// Count returns the number of entries in the log.
func (q *QueryLog) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.count
}
