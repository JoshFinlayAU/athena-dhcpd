// Package audit provides a persistent audit trail for DHCP lease events.
// Records every lease assignment, renewal, release, and expiry with full context.
// Stored in a dedicated BoltDB bucket, separate from operational lease data.
// Queryable by IP+timestamp for compliance (e.g. Telecommunications Act data retention).
package audit

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketAudit   = []byte("audit_log")
	bucketAuditIP = []byte("audit_ip_index") // ip → list of audit record keys
)

// Record is a single audit log entry.
type Record struct {
	ID           uint64 `json:"id"`
	Timestamp    string `json:"timestamp"`
	Event        string `json:"event"`
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	ClientID     string `json:"client_id,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	FQDN         string `json:"fqdn,omitempty"`
	Subnet       string `json:"subnet,omitempty"`
	Pool         string `json:"pool,omitempty"`
	LeaseStart   int64  `json:"lease_start,omitempty"`
	LeaseExpiry  int64  `json:"lease_expiry,omitempty"`
	CircuitID    string `json:"circuit_id,omitempty"`
	RemoteID     string `json:"remote_id,omitempty"`
	GIAddr       string `json:"giaddr,omitempty"`
	ServerID     string `json:"server_id,omitempty"`
	HARoleAtTime string `json:"ha_role,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// QueryParams holds filter parameters for querying the audit log.
type QueryParams struct {
	IP    string    // filter by IP address
	MAC   string    // filter by MAC address
	At    time.Time // point-in-time query: who had this IP at this time?
	From  time.Time // range start (inclusive)
	To    time.Time // range end (inclusive)
	Event string    // filter by event type
	Limit int       // max results (0 = no limit, default 1000)
}

// Log provides append-only audit logging for DHCP lease events.
type Log struct {
	db     *bolt.DB
	bus    *events.Bus
	logger *slog.Logger
	ch     chan events.Event
	done   chan struct{}
	wg     sync.WaitGroup

	serverID string
	haRole   string
	mu       sync.RWMutex
}

// NewLog creates a new audit log backed by BoltDB.
func NewLog(db *bolt.DB, bus *events.Bus, serverID string, logger *slog.Logger) (*Log, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketAudit); err != nil {
			return fmt.Errorf("creating audit bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketAuditIP); err != nil {
			return fmt.Errorf("creating audit IP index: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Log{
		db:       db,
		bus:      bus,
		logger:   logger,
		done:     make(chan struct{}),
		serverID: serverID,
	}, nil
}

// SetHARole updates the current HA role for tagging new records.
func (l *Log) SetHARole(role string) {
	l.mu.Lock()
	l.haRole = role
	l.mu.Unlock()
}

// Start subscribes to the event bus and begins recording audit entries.
func (l *Log) Start() {
	l.ch = l.bus.Subscribe(2000)
	l.logger.Info("audit log started")

	for {
		select {
		case evt, ok := <-l.ch:
			if !ok {
				return
			}
			l.handleEvent(evt)
		case <-l.done:
			return
		}
	}
}

// Stop shuts down the audit log subscriber.
func (l *Log) Stop() {
	close(l.done)
	if l.ch != nil {
		l.bus.Unsubscribe(l.ch)
	}
	l.wg.Wait()
	l.logger.Info("audit log stopped")
}

// handleEvent converts a bus event into an audit record and persists it.
func (l *Log) handleEvent(evt events.Event) {
	// Only audit lease lifecycle events
	switch evt.Type {
	case events.EventLeaseAck, events.EventLeaseRenew,
		events.EventLeaseRelease, events.EventLeaseExpire,
		events.EventLeaseDecline, events.EventLeaseNak:
		// record these
	default:
		return
	}

	if evt.Lease == nil {
		return
	}

	ld := evt.Lease
	l.mu.RLock()
	haRole := l.haRole
	l.mu.RUnlock()

	rec := Record{
		Timestamp:    evt.Timestamp.UTC().Format(time.RFC3339Nano),
		Event:        string(evt.Type),
		IP:           ipStr(ld.IP),
		MAC:          ld.MAC,
		ClientID:     ld.ClientID,
		Hostname:     ld.Hostname,
		FQDN:         ld.FQDN,
		Subnet:       ld.Subnet,
		Pool:         ld.Pool,
		LeaseStart:   ld.Start,
		LeaseExpiry:  ld.Expiry,
		ServerID:     l.serverID,
		HARoleAtTime: haRole,
		Reason:       evt.Reason,
	}

	if ld.Relay != nil {
		rec.CircuitID = ld.Relay.CircuitID
		rec.RemoteID = ld.Relay.RemoteID
		rec.GIAddr = ipStr(ld.Relay.GIAddr)
	}

	if err := l.append(rec); err != nil {
		l.logger.Error("failed to write audit record",
			"event", rec.Event, "ip", rec.IP, "mac", rec.MAC, "error", err)
	}
}

// append persists a single audit record to BoltDB with an auto-increment ID.
func (l *Log) append(rec Record) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAudit)

		// Auto-increment ID
		id, err := b.NextSequence()
		if err != nil {
			return fmt.Errorf("generating audit ID: %w", err)
		}
		rec.ID = id

		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshalling audit record: %w", err)
		}

		key := uint64Key(id)
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("storing audit record: %w", err)
		}

		// Update IP index
		if rec.IP != "" {
			idx := tx.Bucket(bucketAuditIP)
			ipKey := []byte(rec.IP)
			existing := idx.Get(ipKey)
			var ids []uint64
			if existing != nil {
				json.Unmarshal(existing, &ids)
			}
			ids = append(ids, id)
			idData, _ := json.Marshal(ids)
			idx.Put(ipKey, idData)
		}

		return nil
	})
}

// Query searches the audit log with the given parameters.
func (l *Log) Query(params QueryParams) ([]Record, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 1000
	}

	var results []Record

	// Fast path: IP-based query using the index
	if params.IP != "" {
		return l.queryByIP(params, limit)
	}

	// Full scan (filtered by other params)
	err := l.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAudit)
		c := b.Cursor()

		// Iterate newest first (reverse)
		for k, v := c.Last(); k != nil && len(results) < limit; k, v = c.Prev() {
			var rec Record
			if err := json.Unmarshal(v, &rec); err != nil {
				continue
			}
			if matchesQuery(rec, params) {
				results = append(results, rec)
			}
		}
		return nil
	})

	return results, err
}

// queryByIP uses the IP index for efficient lookups.
func (l *Log) queryByIP(params QueryParams, limit int) ([]Record, error) {
	var results []Record

	err := l.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bucketAuditIP)
		b := tx.Bucket(bucketAudit)

		idsData := idx.Get([]byte(params.IP))
		if idsData == nil {
			return nil
		}

		var ids []uint64
		if err := json.Unmarshal(idsData, &ids); err != nil {
			return nil
		}

		// Iterate in reverse (newest first)
		for i := len(ids) - 1; i >= 0 && len(results) < limit; i-- {
			data := b.Get(uint64Key(ids[i]))
			if data == nil {
				continue
			}
			var rec Record
			if err := json.Unmarshal(data, &rec); err != nil {
				continue
			}
			if matchesQuery(rec, params) {
				results = append(results, rec)
			}
		}
		return nil
	})

	return results, err
}

// Count returns the total number of audit records.
func (l *Log) Count() int {
	var count int
	l.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAudit)
		count = b.Stats().KeyN
		return nil
	})
	return count
}

// matchesQuery returns true if a record matches all non-zero query fields.
func matchesQuery(rec Record, params QueryParams) bool {
	if params.MAC != "" && rec.MAC != params.MAC {
		return false
	}
	if params.Event != "" && rec.Event != params.Event {
		return false
	}

	recTime, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if err != nil {
		return false
	}

	// Point-in-time query: who had this IP at this specific time?
	if !params.At.IsZero() {
		// The lease must have started before 'At' and expired after 'At'
		if rec.LeaseStart > params.At.Unix() {
			return false
		}
		if rec.LeaseExpiry != 0 && rec.LeaseExpiry < params.At.Unix() {
			return false
		}
		// Also check the event timestamp is before the query time
		if recTime.After(params.At) {
			return false
		}
		return true
	}

	// Time range filter
	if !params.From.IsZero() && recTime.Before(params.From) {
		return false
	}
	if !params.To.IsZero() && recTime.After(params.To) {
		return false
	}

	return true
}

// --- helpers ---

func uint64Key(id uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, id)
	return key
}

func ipStr(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func macStr(mac net.HardwareAddr) string {
	if mac == nil {
		return ""
	}
	return mac.String()
}
