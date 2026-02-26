// Package macvendor provides MAC address to vendor name lookup using the
// embedded macdb.json database. It loads the OUI database into memory at
// startup and provides O(1) lookups by MAC prefix.
package macvendor

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Entry represents a single MAC vendor database record.
type Entry struct {
	MacPrefix  string `json:"macPrefix"`
	VendorName string `json:"vendorName"`
	Private    bool   `json:"private"`
	BlockType  string `json:"blockType"`
}

// DB is the in-memory MAC vendor database.
type DB struct {
	mu      sync.RWMutex
	vendors map[string]string // normalized prefix -> vendor name
	count   int
}

// NewDB creates a new empty MAC vendor database.
func NewDB() *DB {
	return &DB{
		vendors: make(map[string]string),
	}
}

// Load parses a macdb.json byte slice and loads it into memory.
func (db *DB) Load(data []byte) error {
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing macdb.json: %w", err)
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.vendors = make(map[string]string, len(entries))
	for _, e := range entries {
		prefix := normalizePrefix(e.MacPrefix)
		if prefix != "" {
			db.vendors[prefix] = e.VendorName
		}
	}
	db.count = len(db.vendors)
	return nil
}

// Lookup returns the vendor name for a MAC address, or "" if unknown.
func (db *DB) Lookup(mac string) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	normalized := normalizeMac(mac)
	if len(normalized) < 6 {
		return ""
	}

	// Try longest prefix first (MA-S = 9 chars, MA-M = 7 chars, MA-L = 6 chars)
	for _, prefixLen := range []int{9, 7, 6} {
		if prefixLen > len(normalized) {
			continue
		}
		prefix := normalized[:prefixLen]
		if vendor, ok := db.vendors[prefix]; ok {
			return vendor
		}
	}

	return ""
}

// Count returns the number of vendor entries loaded.
func (db *DB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.count
}

// normalizePrefix converts "00:00:0C" or "00-00-0C" to "00000c" (lowercase hex, no separators).
func normalizePrefix(prefix string) string {
	s := strings.ReplaceAll(prefix, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	return strings.ToLower(s)
}

// normalizeMac converts any MAC format to lowercase hex without separators.
func normalizeMac(mac string) string {
	s := strings.ReplaceAll(mac, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	return strings.ToLower(s)
}
