// Package conflict provides IP conflict detection via ARP/ICMP probing.
package conflict

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketConflicts = []byte("conflicts")

// Record represents an IP address conflict.
type Record struct {
	IP              net.IP    `json:"ip"`
	DetectedAt      time.Time `json:"detected_at"`
	DetectionMethod string    `json:"detection_method"`
	ResponderMAC    string    `json:"responder_mac,omitempty"`
	HoldUntil       time.Time `json:"hold_until"`
	Subnet          string    `json:"subnet"`
	ProbeCount      int       `json:"probe_count"`
	Permanent       bool      `json:"permanent"`
	Resolved        bool      `json:"resolved"`
	ResolvedAt      time.Time `json:"resolved_at,omitempty"`
}

// Table manages the conflict table with BoltDB persistence and in-memory cache.
type Table struct {
	db              *bolt.DB
	records         map[string]*Record // IP string → Record
	mu              sync.RWMutex
	holdTime        time.Duration
	maxConflictCount int
}

// NewTable creates a new conflict table backed by BoltDB.
func NewTable(db *bolt.DB, holdTime time.Duration, maxConflictCount int) (*Table, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketConflicts)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("creating conflicts bucket: %w", err)
	}

	t := &Table{
		db:              db,
		records:         make(map[string]*Record),
		holdTime:        holdTime,
		maxConflictCount: maxConflictCount,
	}

	if err := t.loadAll(); err != nil {
		return nil, fmt.Errorf("loading conflict table: %w", err)
	}

	return t, nil
}

// loadAll reads all conflict records from BoltDB into memory.
func (t *Table) loadAll() error {
	return t.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConflicts)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			r := &Record{}
			if err := json.Unmarshal(v, r); err != nil {
				return fmt.Errorf("unmarshalling conflict %s: %w", k, err)
			}
			t.records[string(k)] = r
			return nil
		})
	})
}

// Add records a new conflict or increments an existing one.
// Returns true if the IP is now permanently flagged.
func (t *Table) Add(ip net.IP, method, responderMAC, subnet string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ipStr := ip.String()
	now := time.Now()

	r, exists := t.records[ipStr]
	if exists {
		r.ProbeCount++
		r.DetectedAt = now
		r.DetectionMethod = method
		r.ResponderMAC = responderMAC
		r.HoldUntil = now.Add(t.holdTime)
		r.Resolved = false
		r.ResolvedAt = time.Time{}

		if r.ProbeCount >= t.maxConflictCount {
			r.Permanent = true
		}
	} else {
		r = &Record{
			IP:              ip,
			DetectedAt:      now,
			DetectionMethod: method,
			ResponderMAC:    responderMAC,
			HoldUntil:       now.Add(t.holdTime),
			Subnet:          subnet,
			ProbeCount:      1,
			Permanent:       false,
			Resolved:        false,
		}
	}

	t.records[ipStr] = r

	if err := t.persist(ipStr, r); err != nil {
		return r.Permanent, fmt.Errorf("persisting conflict for %s: %w", ip, err)
	}

	return r.Permanent, nil
}

// IsConflicted returns true if the IP is currently in the conflict table and not resolved.
func (t *Table) IsConflicted(ip net.IP) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	r, ok := t.records[ip.String()]
	if !ok {
		return false
	}

	if r.Resolved {
		return false
	}

	// If hold time has expired and not permanent, it's eligible again
	if !r.Permanent && time.Now().After(r.HoldUntil) {
		return false
	}

	return true
}

// Get returns the conflict record for an IP, or nil.
func (t *Table) Get(ip net.IP) *Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	r, ok := t.records[ip.String()]
	if !ok {
		return nil
	}
	rc := *r
	return &rc
}

// Resolve marks a conflict as resolved (hold timer expired or manual clear).
func (t *Table) Resolve(ip net.IP) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ipStr := ip.String()
	r, ok := t.records[ipStr]
	if !ok {
		return nil
	}

	r.Resolved = true
	r.ResolvedAt = time.Now()

	if err := t.persist(ipStr, r); err != nil {
		return fmt.Errorf("persisting resolved conflict for %s: %w", ip, err)
	}

	return nil
}

// Clear removes a conflict record entirely (manual admin action).
func (t *Table) Clear(ip net.IP) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ipStr := ip.String()
	delete(t.records, ipStr)

	return t.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConflicts)
		return b.Delete([]byte(ipStr))
	})
}

// AllActive returns all active (unresolved) conflict records.
func (t *Table) AllActive() []*Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var active []*Record
	now := time.Now()
	for _, r := range t.records {
		if r.Resolved {
			continue
		}
		if !r.Permanent && now.After(r.HoldUntil) {
			continue
		}
		rc := *r
		active = append(active, &rc)
	}
	return active
}

// AllResolved returns recently resolved conflicts.
func (t *Table) AllResolved() []*Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var resolved []*Record
	for _, r := range t.records {
		if r.Resolved {
			rc := *r
			resolved = append(resolved, &rc)
		}
	}
	return resolved
}

// Count returns the number of active conflicts.
func (t *Table) Count() int {
	return len(t.AllActive())
}

// PermanentCount returns the number of permanently flagged IPs.
func (t *Table) PermanentCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, r := range t.records {
		if r.Permanent && !r.Resolved {
			count++
		}
	}
	return count
}

// CleanupExpired resolves conflicts whose hold timer has expired.
func (t *Table) CleanupExpired() []net.IP {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var resolved []net.IP

	for ipStr, r := range t.records {
		if r.Resolved || r.Permanent {
			continue
		}
		if now.After(r.HoldUntil) {
			r.Resolved = true
			r.ResolvedAt = now
			_ = t.persist(ipStr, r)
			resolved = append(resolved, r.IP)
		}
	}

	return resolved
}

// persist writes a conflict record to BoltDB.
func (t *Table) persist(ipStr string, r *Record) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return t.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConflicts)
		return b.Put([]byte(ipStr), data)
	})
}

// Records returns all records (for HA sync).
func (t *Table) Records() map[string]*Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*Record, len(t.records))
	for k, v := range t.records {
		rc := *v
		result[k] = &rc
	}
	return result
}
