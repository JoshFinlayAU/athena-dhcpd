package pool

import (
	"path/filepath"
	"strings"
)

// MatchCriteria holds the client attributes used for pool matching.
type MatchCriteria struct {
	CircuitID   string
	RemoteID    string
	VendorClass string
}

// Matches returns true if the pool's match criteria are satisfied by the given client attributes.
// Empty match fields on the pool mean "match all" for that attribute.
// Supports glob patterns (e.g., "eth0/1/*", "Cisco*").
func (p *Pool) Matches(criteria MatchCriteria) bool {
	if p.MatchCircuitID != "" {
		if !matchGlob(p.MatchCircuitID, criteria.CircuitID) {
			return false
		}
	}
	if p.MatchRemoteID != "" {
		if !matchGlob(p.MatchRemoteID, criteria.RemoteID) {
			return false
		}
	}
	if p.MatchVendorClass != "" {
		if !matchGlob(p.MatchVendorClass, criteria.VendorClass) {
			return false
		}
	}
	return true
}

// HasMatchCriteria returns true if the pool has any match constraints.
func (p *Pool) HasMatchCriteria() bool {
	return p.MatchCircuitID != "" || p.MatchRemoteID != "" || p.MatchVendorClass != ""
}

// matchGlob performs glob-style matching. Falls back to prefix match if glob fails.
func matchGlob(pattern, value string) bool {
	if value == "" {
		return false
	}
	// Try filepath.Match (supports *, ?, [])
	matched, err := filepath.Match(pattern, value)
	if err == nil {
		return matched
	}
	// Fallback: simple prefix match for patterns ending in *
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

// SelectPool finds the best matching pool from a list for the given criteria.
// Pools with match criteria that match are preferred over pools without.
// Returns the first match, or the first pool with no criteria if no specific match.
func SelectPool(pools []*Pool, criteria MatchCriteria) *Pool {
	var defaultPool *Pool

	for _, p := range pools {
		if p.HasMatchCriteria() {
			if p.Matches(criteria) {
				return p
			}
		} else if defaultPool == nil {
			defaultPool = p
		}
	}

	return defaultPool
}
