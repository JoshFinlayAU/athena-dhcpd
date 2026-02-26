// Package fingerprint provides DHCP fingerprinting and device classification.
// Extracts fingerprint data from DHCP packets (vendor class option 60,
// parameter request list option 55, hostname patterns) and classifies devices
// using the Fingerbank API or a local heuristic database.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketFingerprints = []byte("fingerprints")    // mac → DeviceInfo
	bucketFPIndex      = []byte("fingerprint_idx") // fingerprint_hash → []mac
)

// DeviceInfo holds classification data for a device.
type DeviceInfo struct {
	MAC             string    `json:"mac"`
	FingerprintHash string    `json:"fingerprint_hash"`
	VendorClass     string    `json:"vendor_class,omitempty"`
	ParamList       string    `json:"param_list,omitempty"`
	Hostname        string    `json:"hostname,omitempty"`
	OUI             string    `json:"oui,omitempty"`
	DeviceType      string    `json:"device_type,omitempty"`
	DeviceName      string    `json:"device_name,omitempty"`
	OS              string    `json:"os,omitempty"`
	Confidence      int       `json:"confidence"`
	Source          string    `json:"source"` // "local", "fingerbank", "oui"
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
}

// RawFingerprint holds the raw DHCP options used for fingerprinting.
type RawFingerprint struct {
	MAC         net.HardwareAddr
	VendorClass string
	ParamList   []byte // raw option 55 bytes
	Hostname    string
}

// Hash returns a stable hash of the fingerprint for deduplication.
func (rf *RawFingerprint) Hash() string {
	h := sha256.New()
	h.Write([]byte(rf.VendorClass))
	h.Write([]byte{0})
	h.Write(rf.ParamList)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

// ParamListString returns the parameter request list as a comma-separated string of option codes.
func (rf *RawFingerprint) ParamListString() string {
	if len(rf.ParamList) == 0 {
		return ""
	}
	parts := make([]string, len(rf.ParamList))
	for i, b := range rf.ParamList {
		parts[i] = fmt.Sprintf("%d", b)
	}
	return strings.Join(parts, ",")
}

// Store provides persistent storage and lookup of device fingerprints.
type Store struct {
	db     *bolt.DB
	logger *slog.Logger
	mu     sync.RWMutex
	cache  map[string]*DeviceInfo // mac → DeviceInfo
}

// NewStore creates a new fingerprint store backed by BoltDB.
func NewStore(db *bolt.DB, logger *slog.Logger) (*Store, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketFingerprints); err != nil {
			return fmt.Errorf("creating fingerprints bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketFPIndex); err != nil {
			return fmt.Errorf("creating fingerprint index: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s := &Store{
		db:     db,
		logger: logger,
		cache:  make(map[string]*DeviceInfo),
	}

	// Load existing fingerprints into cache
	if err := s.loadAll(); err != nil {
		return nil, fmt.Errorf("loading fingerprints: %w", err)
	}

	return s, nil
}

// Record processes a raw fingerprint and stores the device classification.
func (s *Store) Record(fp *RawFingerprint) *DeviceInfo {
	mac := fp.MAC.String()
	now := time.Now()
	hash := fp.Hash()

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.cache[mac]
	if ok && existing.FingerprintHash == hash {
		// Same fingerprint — just update last_seen
		existing.LastSeen = now
		existing.Hostname = fp.Hostname
		s.persist(existing)
		cp := *existing
		return &cp
	}

	// New or changed fingerprint — classify
	info := &DeviceInfo{
		MAC:             mac,
		FingerprintHash: hash,
		VendorClass:     fp.VendorClass,
		ParamList:       fp.ParamListString(),
		Hostname:        fp.Hostname,
		OUI:             ouiFromMAC(fp.MAC),
		FirstSeen:       now,
		LastSeen:        now,
	}

	if ok {
		info.FirstSeen = existing.FirstSeen
	}

	// Local classification
	classify(info, fp)

	s.cache[mac] = info
	s.persist(info)

	cp := *info
	return &cp
}

// Get returns the device info for a MAC address.
func (s *Store) Get(mac string) *DeviceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if info, ok := s.cache[mac]; ok {
		cp := *info
		return &cp
	}
	return nil
}

// All returns all known device fingerprints.
func (s *Store) All() []DeviceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]DeviceInfo, 0, len(s.cache))
	for _, info := range s.cache {
		result = append(result, *info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen.After(result[j].LastSeen)
	})
	return result
}

// Count returns the number of known fingerprints.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// persist writes a DeviceInfo to BoltDB.
func (s *Store) persist(info *DeviceInfo) {
	data, _ := json.Marshal(info)
	s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFingerprints)
		return b.Put([]byte(info.MAC), data)
	})
}

// loadAll loads all fingerprints from BoltDB into the in-memory cache.
func (s *Store) loadAll() error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFingerprints)
		return b.ForEach(func(k, v []byte) error {
			var info DeviceInfo
			if err := json.Unmarshal(v, &info); err == nil {
				s.cache[info.MAC] = &info
			}
			return nil
		})
	})
}

// ouiFromMAC extracts the OUI prefix (first 3 octets) from a MAC address.
func ouiFromMAC(mac net.HardwareAddr) string {
	if len(mac) < 3 {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x", mac[0], mac[1], mac[2])
}

// classify applies local heuristic classification to a device.
func classify(info *DeviceInfo, fp *RawFingerprint) {
	vc := strings.ToLower(fp.VendorClass)
	hn := strings.ToLower(fp.Hostname)

	// Vendor class based classification
	switch {
	case strings.HasPrefix(vc, "msft "):
		info.OS = "Windows"
		info.DeviceType = "computer"
		info.Confidence = 80
		info.Source = "local"
		if strings.Contains(vc, "5.0") {
			info.DeviceName = "Windows 2000/XP"
		} else if strings.Contains(vc, "6.0") {
			info.DeviceName = "Windows Vista/7"
		} else if strings.Contains(vc, "6.1") {
			info.DeviceName = "Windows 7"
		} else if strings.Contains(vc, "6.2") {
			info.DeviceName = "Windows 8"
		} else if strings.Contains(vc, "6.3") {
			info.DeviceName = "Windows 8.1/10"
		} else if strings.Contains(vc, "10.0") {
			info.DeviceName = "Windows 10/11"
		}

	case strings.HasPrefix(vc, "android-dhcp"):
		info.OS = "Android"
		info.DeviceType = "phone"
		info.Confidence = 85
		info.Source = "local"

	case strings.HasPrefix(vc, "dhcpcd"):
		info.OS = "Linux"
		info.DeviceType = "computer"
		info.Confidence = 60
		info.Source = "local"

	case strings.Contains(vc, "udhcp"):
		info.OS = "Linux (embedded)"
		info.DeviceType = "embedded"
		info.Confidence = 50
		info.Source = "local"

	case strings.Contains(vc, "cisco"):
		info.DeviceType = "network"
		info.DeviceName = "Cisco"
		info.Confidence = 90
		info.Source = "local"

	case strings.Contains(vc, "aruba"):
		info.DeviceType = "network"
		info.DeviceName = "Aruba"
		info.Confidence = 90
		info.Source = "local"

	case strings.Contains(vc, "meraki"):
		info.DeviceType = "network"
		info.DeviceName = "Meraki"
		info.Confidence = 90
		info.Source = "local"

	case strings.Contains(vc, "ubnt"), strings.Contains(vc, "ubiquiti"):
		info.DeviceType = "network"
		info.DeviceName = "Ubiquiti"
		info.Confidence = 90
		info.Source = "local"

	case strings.Contains(vc, "fortigate"), strings.Contains(vc, "fortinet"):
		info.DeviceType = "network"
		info.DeviceName = "Fortinet"
		info.Confidence = 90
		info.Source = "local"
	}

	// Hostname-based hints
	if info.DeviceType == "" {
		switch {
		case strings.HasPrefix(hn, "iphone"), strings.HasPrefix(hn, "ipad"):
			info.OS = "iOS/iPadOS"
			info.DeviceType = "phone"
			info.Confidence = 70
			info.Source = "local"

		case strings.HasPrefix(hn, "macbook"), strings.HasPrefix(hn, "imac"), strings.HasPrefix(hn, "mac-"):
			info.OS = "macOS"
			info.DeviceType = "computer"
			info.Confidence = 70
			info.Source = "local"

		case strings.HasPrefix(hn, "android-"), strings.HasPrefix(hn, "galaxy"):
			info.OS = "Android"
			info.DeviceType = "phone"
			info.Confidence = 60
			info.Source = "local"

		case strings.Contains(hn, "printer"), strings.Contains(hn, "hp-"), strings.Contains(hn, "epson"):
			info.DeviceType = "printer"
			info.Confidence = 60
			info.Source = "local"

		case strings.Contains(hn, "-ap-"), strings.Contains(hn, "-sw-"), strings.Contains(hn, "switch"):
			info.DeviceType = "network"
			info.Confidence = 50
			info.Source = "local"

		case strings.Contains(hn, "cam"), strings.Contains(hn, "nvr"), strings.Contains(hn, "hikvision"):
			info.DeviceType = "camera"
			info.Confidence = 50
			info.Source = "local"
		}
	}

	// Parameter request list based hints (option 55 patterns)
	if info.OS == "" && len(fp.ParamList) > 0 {
		paramStr := fp.ParamListString()
		switch {
		case strings.HasPrefix(paramStr, "1,15,3,6,44,46,47,31,33,121,249,43"):
			info.OS = "Windows"
			info.DeviceType = "computer"
			info.Confidence = 50
			info.Source = "local"
		case strings.HasPrefix(paramStr, "1,3,6,15,119,252"):
			info.OS = "macOS/iOS"
			info.Confidence = 50
			info.Source = "local"
		}
	}

	// Fallback
	if info.DeviceType == "" {
		info.DeviceType = "unknown"
		info.Confidence = 0
		info.Source = "local"
	}
}
