package lease

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// BoltDB bucket names.
var (
	bucketLeases     = []byte("leases")
	bucketIndexMAC   = []byte("index_mac")
	bucketIndexCID   = []byte("index_client_id")
	bucketIndexHost  = []byte("index_hostname")
	bucketConflicts  = []byte("conflicts")
	bucketExcluded   = []byte("excluded_ips")
	bucketMeta       = []byte("meta")
	bucketEventLog   = []byte("event_log")
)

// Store provides lease persistence via BoltDB with in-memory indexes for O(1) lookup.
type Store struct {
	db       *bolt.DB
	mu       sync.RWMutex
	byIP     map[string]*Lease            // IP string → Lease
	byMAC    map[string]*Lease            // MAC string → Lease
	byCID    map[string]*Lease            // Client-ID hex → Lease
	byHost   map[string]*Lease            // Hostname → Lease
	seq      uint64
}

// NewStore opens or creates a BoltDB database and initializes the in-memory indexes.
func NewStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		NoSync: false,
	})
	if err != nil {
		return nil, fmt.Errorf("opening lease database %s: %w", path, err)
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketLeases, bucketIndexMAC, bucketIndexCID,
			bucketIndexHost, bucketConflicts, bucketExcluded,
			bucketMeta, bucketEventLog,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("creating bucket %s: %w", b, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing database buckets: %w", err)
	}

	s := &Store{
		db:     db,
		byIP:   make(map[string]*Lease),
		byMAC:  make(map[string]*Lease),
		byCID:  make(map[string]*Lease),
		byHost: make(map[string]*Lease),
	}

	// Load existing leases into memory
	if err := s.loadAll(); err != nil {
		db.Close()
		return nil, fmt.Errorf("loading leases from database: %w", err)
	}

	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// loadAll reads all leases from BoltDB into in-memory indexes.
func (s *Store) loadAll() error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		return b.ForEach(func(k, v []byte) error {
			l := &Lease{}
			if err := json.Unmarshal(v, l); err != nil {
				return fmt.Errorf("unmarshalling lease %s: %w", k, err)
			}
			s.indexLease(l)
			return nil
		})
	})
}

// indexLease adds a lease to all in-memory indexes (caller must hold write lock or be in init).
func (s *Store) indexLease(l *Lease) {
	ipKey := l.IP.String()
	macKey := l.MAC.String()

	s.byIP[ipKey] = l
	s.byMAC[macKey] = l
	if l.ClientID != "" {
		s.byCID[l.ClientID] = l
	}
	if l.Hostname != "" {
		s.byHost[l.Hostname] = l
	}
}

// unindexLease removes a lease from all in-memory indexes.
func (s *Store) unindexLease(l *Lease) {
	ipKey := l.IP.String()
	macKey := l.MAC.String()

	delete(s.byIP, ipKey)
	delete(s.byMAC, macKey)
	if l.ClientID != "" {
		delete(s.byCID, l.ClientID)
	}
	if l.Hostname != "" {
		delete(s.byHost, l.Hostname)
	}
}

// Put creates or updates a lease in both BoltDB and in-memory indexes.
func (s *Store) Put(l *Lease) error {
	data, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("marshalling lease for %s: %w", l.IP, err)
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		ipKey := []byte(l.IP.String())
		if err := b.Put(ipKey, data); err != nil {
			return fmt.Errorf("writing lease for %s: %w", l.IP, err)
		}

		// Update MAC index
		macBucket := tx.Bucket(bucketIndexMAC)
		if err := macBucket.Put([]byte(l.MAC.String()), ipKey); err != nil {
			return fmt.Errorf("updating MAC index for %s: %w", l.MAC, err)
		}

		// Update client-id index
		if l.ClientID != "" {
			cidBucket := tx.Bucket(bucketIndexCID)
			if err := cidBucket.Put([]byte(l.ClientID), ipKey); err != nil {
				return fmt.Errorf("updating client-id index for %s: %w", l.ClientID, err)
			}
		}

		// Update hostname index
		if l.Hostname != "" {
			hostBucket := tx.Bucket(bucketIndexHost)
			if err := hostBucket.Put([]byte(l.Hostname), ipKey); err != nil {
				return fmt.Errorf("updating hostname index for %s: %w", l.Hostname, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	// Remove old lease for this MAC if IP changed
	if old, ok := s.byMAC[l.MAC.String()]; ok && !old.IP.Equal(l.IP) {
		s.unindexLease(old)
	}
	s.indexLease(l)
	s.mu.Unlock()

	return nil
}

// Delete removes a lease by IP.
func (s *Store) Delete(ip net.IP) error {
	ipStr := ip.String()

	s.mu.RLock()
	l, exists := s.byIP[ipStr]
	s.mu.RUnlock()

	if !exists {
		return nil
	}

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		if err := b.Delete([]byte(ipStr)); err != nil {
			return fmt.Errorf("deleting lease for %s: %w", ip, err)
		}

		macBucket := tx.Bucket(bucketIndexMAC)
		_ = macBucket.Delete([]byte(l.MAC.String()))

		if l.ClientID != "" {
			cidBucket := tx.Bucket(bucketIndexCID)
			_ = cidBucket.Delete([]byte(l.ClientID))
		}
		if l.Hostname != "" {
			hostBucket := tx.Bucket(bucketIndexHost)
			_ = hostBucket.Delete([]byte(l.Hostname))
		}

		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.unindexLease(l)
	s.mu.Unlock()

	return nil
}

// GetByIP returns a lease by IP address. O(1) via in-memory index.
func (s *Store) GetByIP(ip net.IP) *Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.byIP[ip.String()]
	if !ok {
		return nil
	}
	return l.Clone()
}

// GetByMAC returns a lease by MAC address. O(1) via in-memory index.
func (s *Store) GetByMAC(mac net.HardwareAddr) *Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.byMAC[mac.String()]
	if !ok {
		return nil
	}
	return l.Clone()
}

// GetByClientID returns a lease by client identifier. O(1) via in-memory index.
func (s *Store) GetByClientID(clientID string) *Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.byCID[clientID]
	if !ok {
		return nil
	}
	return l.Clone()
}

// GetByHostname returns a lease by hostname. O(1) via in-memory index.
func (s *Store) GetByHostname(hostname string) *Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.byHost[hostname]
	if !ok {
		return nil
	}
	return l.Clone()
}

// All returns all active leases (cloned).
func (s *Store) All() []*Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	leases := make([]*Lease, 0, len(s.byIP))
	for _, l := range s.byIP {
		leases = append(leases, l.Clone())
	}
	return leases
}

// Count returns the total number of leases.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byIP)
}

// ForEach iterates over all leases with a callback. Holds read lock.
func (s *Store) ForEach(fn func(*Lease) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.byIP {
		if !fn(l) {
			return
		}
	}
}

// NextSeq returns and increments the monotonic sequence counter.
func (s *Store) NextSeq() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return s.seq
}

// DB returns the underlying BoltDB instance (for conflict table, etc.).
func (s *Store) DB() *bolt.DB {
	return s.db
}
