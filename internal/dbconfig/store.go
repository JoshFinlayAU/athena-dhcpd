// Package dbconfig provides BoltDB-backed storage for dynamic configuration.
// Bootstrap config (server + api) stays in TOML; everything else lives in the DB.
package dbconfig

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	bolt "go.etcd.io/bbolt"
)

// BoltDB bucket names for config storage.
var (
	bucketSubnets     = []byte("config_subnets")
	bucketDefaults    = []byte("config_defaults")
	bucketConflict    = []byte("config_conflict")
	bucketHooks       = []byte("config_hooks")
	bucketDDNS        = []byte("config_ddns")
	bucketDNS         = []byte("config_dns")
	bucketHostSanit   = []byte("config_hostname_sanitisation")
	bucketFingerprint = []byte("config_fingerprint")
	bucketSyslog      = []byte("config_syslog")
	bucketPortAuto    = []byte("config_portauto")
	bucketVIPs        = []byte("config_vips")
	bucketMeta        = []byte("config_meta")
	bucketUsers       = []byte("config_users")

	keyDefaults      = []byte("defaults")
	keyConflict      = []byte("conflict_detection")
	keyHooks         = []byte("hooks")
	keyDDNS          = []byte("ddns")
	keyDNS           = []byte("dns")
	keyHostSanit     = []byte("hostname_sanitisation")
	keyFingerprint   = []byte("fingerprint")
	keySyslog        = []byte("syslog")
	keyPortAuto      = []byte("portauto_rules")
	keyVIPs          = []byte("vips")
	keySetupComplete = []byte("setup_complete")
)

// Store provides CRUD access to dynamic configuration stored in BoltDB.
// Thread-safe — all reads and writes are protected by a mutex.
type Store struct {
	db *bolt.DB
	mu sync.RWMutex

	// In-memory cache
	subnets       []config.SubnetConfig
	defaults      config.DefaultsConfig
	conflict      config.ConflictDetectionConfig
	hooks         config.HooksConfig
	ddns          config.DDNSConfig
	dns           config.DNSProxyConfig
	hostSanit     config.HostnameSanitisationConfig
	fingerprint   config.FingerprintConfig
	syslog        config.SyslogConfig
	portAutoRules json.RawMessage
	vips          json.RawMessage
	users         []config.UserConfig

	// Listeners notified on config changes (fires for ALL changes, local + peer)
	onChange []func()
	// Listeners notified only for local changes (used to trigger HA peer sync)
	onLocalChange []func(section string, data []byte)

	// Debounce: coalesce rapid config changes into a single onChange callback.
	// Prevents 7 concurrent rebuilds when HA primary pushes 7 config sections.
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
}

// NewStore initializes the config store, creating buckets and loading cached state.
func NewStore(db *bolt.DB) (*Store, error) {
	// Create buckets
	err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketSubnets, bucketDefaults, bucketConflict,
			bucketHooks, bucketDDNS, bucketDNS, bucketHostSanit, bucketFingerprint, bucketSyslog, bucketPortAuto, bucketVIPs, bucketMeta, bucketUsers,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("creating config bucket %s: %w", b, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initializing config buckets: %w", err)
	}

	s := &Store{db: db}
	if err := s.loadAll(); err != nil {
		return nil, fmt.Errorf("loading config from db: %w", err)
	}
	return s, nil
}

// OnChange registers a callback that fires whenever config is modified.
func (s *Store) OnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = append(s.onChange, fn)
}

func (s *Store) notifyChange() {
	s.debounceMu.Lock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
		for _, fn := range s.onChange {
			fn()
		}
	})
	s.debounceMu.Unlock()
}

func (s *Store) notifyLocalChange(section string, data []byte) {
	s.notifyChange()
	for _, fn := range s.onLocalChange {
		go fn(section, data)
	}
}

// OnLocalChange registers a callback that fires only for local config changes
// (not peer-replicated). Used by HA to send config to the partner.
func (s *Store) OnLocalChange(fn func(section string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLocalChange = append(s.onLocalChange, fn)
}

// IsSetupComplete returns true if the initial setup wizard has been completed.
func (s *Store) IsSetupComplete() bool {
	var complete bool
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		if b == nil {
			return nil
		}
		complete = b.Get(keySetupComplete) != nil
		return nil
	})
	return complete
}

// MarkSetupComplete marks the initial setup wizard as done.
func (s *Store) MarkSetupComplete() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		return b.Put(keySetupComplete, []byte("1"))
	})
}

// --- Subnets ---

// Subnets returns all configured subnets.
func (s *Store) Subnets() []config.SubnetConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]config.SubnetConfig, len(s.subnets))
	copy(out, s.subnets)
	return out
}

// GetSubnet returns a subnet by network CIDR.
func (s *Store) GetSubnet(network string) (*config.SubnetConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.subnets {
		if s.subnets[i].Network == network {
			sub := s.subnets[i]
			return &sub, true
		}
	}
	return nil, false
}

// PutSubnet creates or updates a subnet.
func (s *Store) PutSubnet(sub config.SubnetConfig) error {
	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("marshalling subnet: %w", err)
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubnets)
		return b.Put([]byte(sub.Network), data)
	}); err != nil {
		return fmt.Errorf("storing subnet %s: %w", sub.Network, err)
	}

	s.mu.Lock()
	found := false
	for i := range s.subnets {
		if s.subnets[i].Network == sub.Network {
			s.subnets[i] = sub
			found = true
			break
		}
	}
	if !found {
		s.subnets = append(s.subnets, sub)
	}
	snapshot, _ := json.Marshal(s.subnets)
	s.mu.Unlock()
	s.notifyLocalChange("subnets", snapshot)
	return nil
}

// DeleteSubnet removes a subnet by network CIDR.
func (s *Store) DeleteSubnet(network string) error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubnets)
		return b.Delete([]byte(network))
	}); err != nil {
		return fmt.Errorf("deleting subnet %s: %w", network, err)
	}

	s.mu.Lock()
	for i := range s.subnets {
		if s.subnets[i].Network == network {
			s.subnets = append(s.subnets[:i], s.subnets[i+1:]...)
			break
		}
	}
	snapshot, _ := json.Marshal(s.subnets)
	s.mu.Unlock()
	s.notifyLocalChange("subnets", snapshot)
	return nil
}

// --- Reservations (scoped to a subnet) ---

// GetReservations returns all reservations for a subnet.
func (s *Store) GetReservations(network string) []config.ReservationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subnets {
		if sub.Network == network {
			out := make([]config.ReservationConfig, len(sub.Reservations))
			copy(out, sub.Reservations)
			return out
		}
	}
	return nil
}

// PutReservation adds or updates a reservation within a subnet (matched by MAC or Identifier).
func (s *Store) PutReservation(network string, res config.ReservationConfig) error {
	s.mu.Lock()
	var found bool
	for i := range s.subnets {
		if s.subnets[i].Network != network {
			continue
		}
		// Update existing by MAC match
		for j := range s.subnets[i].Reservations {
			if s.subnets[i].Reservations[j].MAC == res.MAC && res.MAC != "" {
				s.subnets[i].Reservations[j] = res
				found = true
				break
			}
		}
		if !found {
			s.subnets[i].Reservations = append(s.subnets[i].Reservations, res)
		}
		// Persist the whole subnet
		s.mu.Unlock()
		return s.PutSubnet(s.subnets[i])
	}
	s.mu.Unlock()
	return fmt.Errorf("subnet %s not found", network)
}

// DeleteReservation removes a reservation by MAC within a subnet.
func (s *Store) DeleteReservation(network, mac string) error {
	s.mu.Lock()
	for i := range s.subnets {
		if s.subnets[i].Network != network {
			continue
		}
		for j := range s.subnets[i].Reservations {
			if s.subnets[i].Reservations[j].MAC == mac {
				s.subnets[i].Reservations = append(
					s.subnets[i].Reservations[:j],
					s.subnets[i].Reservations[j+1:]...,
				)
				s.mu.Unlock()
				return s.PutSubnet(s.subnets[i])
			}
		}
		s.mu.Unlock()
		return fmt.Errorf("reservation with MAC %s not found in subnet %s", mac, network)
	}
	s.mu.Unlock()
	return fmt.Errorf("subnet %s not found", network)
}

// ImportReservations bulk-imports reservations into a subnet (used for CSV import).
func (s *Store) ImportReservations(network string, reservations []config.ReservationConfig) (int, error) {
	s.mu.Lock()
	for i := range s.subnets {
		if s.subnets[i].Network != network {
			continue
		}
		existing := make(map[string]int) // MAC → index
		for j, r := range s.subnets[i].Reservations {
			if r.MAC != "" {
				existing[r.MAC] = j
			}
		}
		added := 0
		for _, r := range reservations {
			if idx, ok := existing[r.MAC]; ok {
				s.subnets[i].Reservations[idx] = r // update
			} else {
				s.subnets[i].Reservations = append(s.subnets[i].Reservations, r)
				added++
			}
		}
		sub := s.subnets[i]
		s.mu.Unlock()
		if err := s.PutSubnet(sub); err != nil {
			return 0, err
		}
		return added, nil
	}
	s.mu.Unlock()
	return 0, fmt.Errorf("subnet %s not found", network)
}

// --- Singleton config sections ---

func (s *Store) Defaults() config.DefaultsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaults
}

func (s *Store) SetDefaults(d config.DefaultsConfig) error {
	data, _ := json.Marshal(d)
	if err := s.putJSON(bucketDefaults, keyDefaults, d); err != nil {
		return err
	}
	s.mu.Lock()
	s.defaults = d
	s.mu.Unlock()
	s.notifyLocalChange("defaults", data)
	return nil
}

func (s *Store) ConflictDetection() config.ConflictDetectionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.conflict
}

func (s *Store) SetConflictDetection(c config.ConflictDetectionConfig) error {
	data, _ := json.Marshal(c)
	if err := s.putJSON(bucketConflict, keyConflict, c); err != nil {
		return err
	}
	s.mu.Lock()
	s.conflict = c
	s.mu.Unlock()
	s.notifyLocalChange("conflict_detection", data)
	return nil
}

func (s *Store) Hooks() config.HooksConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hooks
}

func (s *Store) SetHooks(h config.HooksConfig) error {
	data, _ := json.Marshal(h)
	if err := s.putJSON(bucketHooks, keyHooks, h); err != nil {
		return err
	}
	s.mu.Lock()
	s.hooks = h
	s.mu.Unlock()
	s.notifyLocalChange("hooks", data)
	return nil
}

func (s *Store) DDNS() config.DDNSConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ddns
}

func (s *Store) SetDDNS(d config.DDNSConfig) error {
	data, _ := json.Marshal(d)
	if err := s.putJSON(bucketDDNS, keyDDNS, d); err != nil {
		return err
	}
	s.mu.Lock()
	s.ddns = d
	s.mu.Unlock()
	s.notifyLocalChange("ddns", data)
	return nil
}

func (s *Store) DNS() config.DNSProxyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dns
}

func (s *Store) SetDNS(d config.DNSProxyConfig) error {
	data, _ := json.Marshal(d)
	if err := s.putJSON(bucketDNS, keyDNS, d); err != nil {
		return err
	}
	s.mu.Lock()
	s.dns = d
	s.mu.Unlock()
	s.notifyLocalChange("dns", data)
	return nil
}

func (s *Store) HostnameSanitisation() config.HostnameSanitisationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hostSanit
}

func (s *Store) SetHostnameSanitisation(h config.HostnameSanitisationConfig) error {
	data, _ := json.Marshal(h)
	if err := s.putJSON(bucketHostSanit, keyHostSanit, h); err != nil {
		return err
	}
	s.mu.Lock()
	s.hostSanit = h
	s.mu.Unlock()
	s.notifyLocalChange("hostname_sanitisation", data)
	return nil
}

func (s *Store) Fingerprint() config.FingerprintConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fingerprint
}

func (s *Store) SetFingerprint(f config.FingerprintConfig) error {
	data, _ := json.Marshal(f)
	if err := s.putJSON(bucketFingerprint, keyFingerprint, f); err != nil {
		return err
	}
	s.mu.Lock()
	s.fingerprint = f
	s.mu.Unlock()
	s.notifyLocalChange("fingerprint", data)
	return nil
}

func (s *Store) Syslog() config.SyslogConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syslog
}

func (s *Store) SetSyslog(sl config.SyslogConfig) error {
	data, _ := json.Marshal(sl)
	if err := s.putJSON(bucketSyslog, keySyslog, sl); err != nil {
		return err
	}
	s.mu.Lock()
	s.syslog = sl
	s.mu.Unlock()
	s.notifyLocalChange("syslog", data)
	return nil
}

// --- Port Automation Rules (stored as raw JSON) ---

func (s *Store) PortAutoRules() json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.portAutoRules == nil {
		return nil
	}
	cp := make(json.RawMessage, len(s.portAutoRules))
	copy(cp, s.portAutoRules)
	return cp
}

func (s *Store) SetPortAutoRules(data json.RawMessage) error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketPortAuto).Put(keyPortAuto, data)
	}); err != nil {
		return fmt.Errorf("storing portauto rules: %w", err)
	}
	s.mu.Lock()
	s.portAutoRules = make(json.RawMessage, len(data))
	copy(s.portAutoRules, data)
	s.mu.Unlock()
	s.notifyLocalChange("portauto", data)
	return nil
}

// --- Virtual IPs (floating IPs for HA) ---

func (s *Store) VIPs() json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.vips == nil {
		return nil
	}
	cp := make(json.RawMessage, len(s.vips))
	copy(cp, s.vips)
	return cp
}

func (s *Store) SetVIPs(data json.RawMessage) error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketVIPs).Put(keyVIPs, data)
	}); err != nil {
		return fmt.Errorf("storing VIPs: %w", err)
	}
	s.mu.Lock()
	s.vips = make(json.RawMessage, len(data))
	copy(s.vips, data)
	s.mu.Unlock()
	s.notifyLocalChange("vips", data)
	return nil
}

// HA config lives in TOML, not the database — see config.WriteHASection().

// --- Build full config ---

// BuildConfig merges bootstrap TOML config with DB-stored dynamic config.
func (s *Store) BuildConfig(bootstrap *config.Config) *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := *bootstrap // shallow copy
	cfg.Subnets = make([]config.SubnetConfig, len(s.subnets))
	copy(cfg.Subnets, s.subnets)
	cfg.Defaults = s.defaults
	cfg.ConflictDetection = s.conflict
	// HA stays in TOML — it's node-identity config outside the DB sync
	cfg.Hooks = s.hooks
	cfg.DDNS = s.ddns
	cfg.DNS = s.dns
	cfg.HostnameSanitisation = s.hostSanit
	cfg.Fingerprint = s.fingerprint
	cfg.Syslog = s.syslog

	// Merge DB users into API auth (TOML users take precedence by being first)
	if len(s.users) > 0 {
		existing := make(map[string]bool, len(cfg.API.Auth.Users))
		for _, u := range cfg.API.Auth.Users {
			existing[u.Username] = true
		}
		for _, u := range s.users {
			if !existing[u.Username] {
				cfg.API.Auth.Users = append(cfg.API.Auth.Users, u)
			}
		}
	}

	return &cfg
}

// ImportFromConfig imports all dynamic sections from a full config (v1 TOML migration).
func (s *Store) ImportFromConfig(cfg *config.Config) error {
	if err := s.SetDefaults(cfg.Defaults); err != nil {
		return fmt.Errorf("importing defaults: %w", err)
	}
	if err := s.SetConflictDetection(cfg.ConflictDetection); err != nil {
		return fmt.Errorf("importing conflict detection: %w", err)
	}
	// HA is bootstrap config — not imported to DB
	if err := s.SetHooks(cfg.Hooks); err != nil {
		return fmt.Errorf("importing hooks: %w", err)
	}
	if err := s.SetDDNS(cfg.DDNS); err != nil {
		return fmt.Errorf("importing DDNS: %w", err)
	}
	if err := s.SetDNS(cfg.DNS); err != nil {
		return fmt.Errorf("importing DNS: %w", err)
	}
	if err := s.SetHostnameSanitisation(cfg.HostnameSanitisation); err != nil {
		return fmt.Errorf("importing hostname sanitisation: %w", err)
	}
	if err := s.SetFingerprint(cfg.Fingerprint); err != nil {
		return fmt.Errorf("importing fingerprint: %w", err)
	}
	if err := s.SetSyslog(cfg.Syslog); err != nil {
		return fmt.Errorf("importing syslog: %w", err)
	}
	for _, sub := range cfg.Subnets {
		if err := s.PutSubnet(sub); err != nil {
			return fmt.Errorf("importing subnet %s: %w", sub.Network, err)
		}
	}
	return nil
}

// --- HA Config Export ---

// ExportAllSections returns a map of section name → JSON data for full config sync.
func (s *Store) ExportAllSections() map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sections := make(map[string][]byte, 6)
	if data, err := json.Marshal(s.subnets); err == nil {
		sections["subnets"] = data
	}
	if data, err := json.Marshal(s.defaults); err == nil {
		sections["defaults"] = data
	}
	if data, err := json.Marshal(s.conflict); err == nil {
		sections["conflict_detection"] = data
	}
	// HA is bootstrap config — not synced between peers
	if data, err := json.Marshal(s.hooks); err == nil {
		sections["hooks"] = data
	}
	if data, err := json.Marshal(s.ddns); err == nil {
		sections["ddns"] = data
	}
	if data, err := json.Marshal(s.dns); err == nil {
		sections["dns"] = data
	}
	if data, err := json.Marshal(s.hostSanit); err == nil {
		sections["hostname_sanitisation"] = data
	}
	if data, err := json.Marshal(s.fingerprint); err == nil {
		sections["fingerprint"] = data
	}
	if data, err := json.Marshal(s.syslog); err == nil {
		sections["syslog"] = data
	}
	if s.portAutoRules != nil {
		sections["portauto"] = s.portAutoRules
	}
	if s.vips != nil {
		sections["vips"] = s.vips
	}
	return sections
}

// --- HA Peer Config Sync ---

// ApplyPeerConfig applies a config section received from the HA peer.
// It persists to BoltDB, updates the in-memory cache, and fires onChange
// (for local reload) but NOT onLocalChange (to avoid echoing back to peer).
func (s *Store) ApplyPeerConfig(section string, data []byte) error {
	switch section {
	case "subnets":
		var subs []config.SubnetConfig
		if err := json.Unmarshal(data, &subs); err != nil {
			return fmt.Errorf("unmarshalling peer subnets: %w", err)
		}
		// Replace all subnets in BoltDB
		if err := s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucketSubnets)
			// Clear existing
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
			// Write new
			for _, sub := range subs {
				d, err := json.Marshal(sub)
				if err != nil {
					return err
				}
				if err := b.Put([]byte(sub.Network), d); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("applying peer subnets: %w", err)
		}
		s.mu.Lock()
		s.subnets = subs
		s.mu.Unlock()

	case "defaults":
		var d config.DefaultsConfig
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("unmarshalling peer defaults: %w", err)
		}
		if err := s.putJSON(bucketDefaults, keyDefaults, d); err != nil {
			return err
		}
		s.mu.Lock()
		s.defaults = d
		s.mu.Unlock()

	case "conflict_detection":
		var c config.ConflictDetectionConfig
		if err := json.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("unmarshalling peer conflict config: %w", err)
		}
		if err := s.putJSON(bucketConflict, keyConflict, c); err != nil {
			return err
		}
		s.mu.Lock()
		s.conflict = c
		s.mu.Unlock()

	case "hooks":
		var h config.HooksConfig
		if err := json.Unmarshal(data, &h); err != nil {
			return fmt.Errorf("unmarshalling peer hooks config: %w", err)
		}
		if err := s.putJSON(bucketHooks, keyHooks, h); err != nil {
			return err
		}
		s.mu.Lock()
		s.hooks = h
		s.mu.Unlock()

	case "ddns":
		var d config.DDNSConfig
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("unmarshalling peer DDNS config: %w", err)
		}
		if err := s.putJSON(bucketDDNS, keyDDNS, d); err != nil {
			return err
		}
		s.mu.Lock()
		s.ddns = d
		s.mu.Unlock()

	case "dns":
		var d config.DNSProxyConfig
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("unmarshalling peer DNS config: %w", err)
		}
		if err := s.putJSON(bucketDNS, keyDNS, d); err != nil {
			return err
		}
		s.mu.Lock()
		s.dns = d
		s.mu.Unlock()

	case "hostname_sanitisation":
		var h config.HostnameSanitisationConfig
		if err := json.Unmarshal(data, &h); err != nil {
			return fmt.Errorf("unmarshalling peer hostname sanitisation config: %w", err)
		}
		if err := s.putJSON(bucketHostSanit, keyHostSanit, h); err != nil {
			return err
		}
		s.mu.Lock()
		s.hostSanit = h
		s.mu.Unlock()

	case "fingerprint":
		var f config.FingerprintConfig
		if err := json.Unmarshal(data, &f); err != nil {
			return fmt.Errorf("unmarshalling peer fingerprint config: %w", err)
		}
		if err := s.putJSON(bucketFingerprint, keyFingerprint, f); err != nil {
			return err
		}
		s.mu.Lock()
		s.fingerprint = f
		s.mu.Unlock()

	case "syslog":
		var sl config.SyslogConfig
		if err := json.Unmarshal(data, &sl); err != nil {
			return fmt.Errorf("unmarshalling peer syslog config: %w", err)
		}
		if err := s.putJSON(bucketSyslog, keySyslog, sl); err != nil {
			return err
		}
		s.mu.Lock()
		s.syslog = sl
		s.mu.Unlock()

	case "portauto":
		// Validate it's valid JSON array
		if !json.Valid(data) {
			return fmt.Errorf("invalid JSON for peer portauto rules")
		}
		if err := s.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(bucketPortAuto).Put(keyPortAuto, data)
		}); err != nil {
			return fmt.Errorf("storing peer portauto rules: %w", err)
		}
		s.mu.Lock()
		s.portAutoRules = make(json.RawMessage, len(data))
		copy(s.portAutoRules, data)
		s.mu.Unlock()

	case "vips":
		if !json.Valid(data) {
			return fmt.Errorf("invalid JSON for peer VIPs")
		}
		if err := s.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(bucketVIPs).Put(keyVIPs, data)
		}); err != nil {
			return fmt.Errorf("storing peer VIPs: %w", err)
		}
		s.mu.Lock()
		s.vips = make(json.RawMessage, len(data))
		copy(s.vips, data)
		s.mu.Unlock()

	default:
		return fmt.Errorf("unknown config section from peer: %s", section)
	}

	// Fire onChange only (triggers local reload) — no onLocalChange (no echo back)
	s.notifyChange()
	return nil
}

// --- Internal helpers ---

func (s *Store) putJSON(bucket, key []byte, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		return b.Put(key, data)
	})
}

func (s *Store) loadAll() error {
	return s.db.View(func(tx *bolt.Tx) error {
		// Load subnets
		b := tx.Bucket(bucketSubnets)
		if b != nil {
			b.ForEach(func(k, v []byte) error {
				var sub config.SubnetConfig
				if err := json.Unmarshal(v, &sub); err == nil {
					s.subnets = append(s.subnets, sub)
				}
				return nil
			})
		}

		// Load singleton sections
		loadJSON(tx, bucketDefaults, keyDefaults, &s.defaults)
		loadJSON(tx, bucketConflict, keyConflict, &s.conflict)
		loadJSON(tx, bucketHooks, keyHooks, &s.hooks)
		loadJSON(tx, bucketDDNS, keyDDNS, &s.ddns)
		loadJSON(tx, bucketDNS, keyDNS, &s.dns)
		loadJSON(tx, bucketHostSanit, keyHostSanit, &s.hostSanit)
		loadJSON(tx, bucketFingerprint, keyFingerprint, &s.fingerprint)
		loadJSON(tx, bucketSyslog, keySyslog, &s.syslog)

		// Load portauto rules as raw JSON
		pab := tx.Bucket(bucketPortAuto)
		if pab != nil {
			if data := pab.Get(keyPortAuto); data != nil {
				s.portAutoRules = make(json.RawMessage, len(data))
				copy(s.portAutoRules, data)
			}
		}

		// Load VIPs as raw JSON
		vb := tx.Bucket(bucketVIPs)
		if vb != nil {
			if data := vb.Get(keyVIPs); data != nil {
				s.vips = make(json.RawMessage, len(data))
				copy(s.vips, data)
			}
		}

		// HA config lives in TOML, not the database — see config.WriteHASection()

		// Load users
		ub := tx.Bucket(bucketUsers)
		if ub != nil {
			ub.ForEach(func(k, v []byte) error {
				var u config.UserConfig
				if err := json.Unmarshal(v, &u); err == nil {
					s.users = append(s.users, u)
				}
				return nil
			})
		}

		return nil
	})
}

// --- User management ---

// Users returns all configured users.
func (s *Store) Users() []config.UserConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]config.UserConfig, len(s.users))
	copy(out, s.users)
	return out
}

// PutUser creates or updates a user in the database.
func (s *Store) PutUser(u config.UserConfig) error {
	data, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("marshalling user: %w", err)
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUsers).Put([]byte(u.Username), data)
	}); err != nil {
		return fmt.Errorf("storing user %s: %w", u.Username, err)
	}

	s.mu.Lock()
	found := false
	for i, existing := range s.users {
		if existing.Username == u.Username {
			s.users[i] = u
			found = true
			break
		}
	}
	if !found {
		s.users = append(s.users, u)
	}
	s.mu.Unlock()

	s.notifyLocalChange("users", data)
	return nil
}

// DeleteUser removes a user from the database.
func (s *Store) DeleteUser(username string) error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUsers).Delete([]byte(username))
	}); err != nil {
		return fmt.Errorf("deleting user %s: %w", username, err)
	}

	s.mu.Lock()
	for i, u := range s.users {
		if u.Username == username {
			s.users = append(s.users[:i], s.users[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	s.notifyLocalChange("users", nil)
	return nil
}

// HasUsers returns true if at least one user is configured in the DB.
func (s *Store) HasUsers() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users) > 0
}

func loadJSON(tx *bolt.Tx, bucket, key []byte, dest interface{}) {
	b := tx.Bucket(bucket)
	if b == nil {
		return
	}
	data := b.Get(key)
	if data == nil {
		return
	}
	json.Unmarshal(data, dest)
}
