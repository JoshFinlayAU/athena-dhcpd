package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
)

// handleHealth returns server health status (no auth required).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"leases":    s.leaseStore.Count(),
	})
}

// leaseResponse is the JSON representation of a lease.
type leaseResponse struct {
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	ClientID    string `json:"client_id,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Subnet      string `json:"subnet"`
	Pool        string `json:"pool,omitempty"`
	State       string `json:"state"`
	Start       int64  `json:"start"`
	Expiry      int64  `json:"expiry"`
	Remaining   int64  `json:"remaining_seconds"`
	LastUpdated int64  `json:"last_updated"`
}

// handleListLeases returns all leases with optional filtering.
// Query params: subnet, mac, hostname, state, limit, offset
func (s *Server) handleListLeases(w http.ResponseWriter, r *http.Request) {
	leases := s.leaseStore.All()

	// Apply filters
	subnetFilter := r.URL.Query().Get("subnet")
	macFilter := strings.ToLower(r.URL.Query().Get("mac"))
	hostnameFilter := strings.ToLower(r.URL.Query().Get("hostname"))
	stateFilter := r.URL.Query().Get("state")

	var filtered []*lease.Lease
	for _, l := range leases {
		if subnetFilter != "" && l.Subnet != subnetFilter {
			continue
		}
		if macFilter != "" && !strings.Contains(strings.ToLower(l.MAC.String()), macFilter) {
			continue
		}
		if hostnameFilter != "" && !strings.Contains(strings.ToLower(l.Hostname), hostnameFilter) {
			continue
		}
		if stateFilter != "" && string(l.State) != stateFilter {
			continue
		}
		filtered = append(filtered, l)
	}

	// Apply pagination
	total := len(filtered)
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	offset := 0
	if offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v > 0 {
			offset = v
		}
	}
	limit := total
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}

	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	paged := filtered[offset:end]

	result := make([]leaseResponse, 0, len(paged))
	for _, l := range paged {
		result = append(result, leaseToResponse(l))
	}

	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	JSONResponse(w, http.StatusOK, result)
}

// handleExportLeases exports leases as CSV.
func (s *Server) handleExportLeases(w http.ResponseWriter, r *http.Request) {
	leases := s.leaseStore.All()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=leases.csv")

	cw := csv.NewWriter(w)
	cw.Write([]string{"ip", "mac", "client_id", "hostname", "subnet", "pool", "state", "start", "expiry", "remaining_seconds"})

	for _, l := range leases {
		remaining := time.Until(l.Expiry).Seconds()
		if remaining < 0 {
			remaining = 0
		}
		cw.Write([]string{
			l.IP.String(),
			l.MAC.String(),
			l.ClientID,
			l.Hostname,
			l.Subnet,
			l.Pool,
			string(l.State),
			l.Start.Format(time.RFC3339),
			l.Expiry.Format(time.RFC3339),
			fmt.Sprintf("%.0f", remaining),
		})
	}
	cw.Flush()
}

// leaseToResponse converts a Lease to the API response format.
func leaseToResponse(l *lease.Lease) leaseResponse {
	remaining := time.Until(l.Expiry).Seconds()
	if remaining < 0 {
		remaining = 0
	}
	return leaseResponse{
		IP:          l.IP.String(),
		MAC:         l.MAC.String(),
		ClientID:    l.ClientID,
		Hostname:    l.Hostname,
		Subnet:      l.Subnet,
		Pool:        l.Pool,
		State:       string(l.State),
		Start:       l.Start.Unix(),
		Expiry:      l.Expiry.Unix(),
		Remaining:   int64(remaining),
		LastUpdated: l.LastUpdated.Unix(),
	}
}

// handleGetLease returns a single lease by IP.
func (s *Server) handleGetLease(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
		return
	}

	l := s.leaseStore.GetByIP(ip)
	if l == nil {
		JSONError(w, http.StatusNotFound, "not_found", "lease not found")
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"ip":           l.IP.String(),
		"mac":          l.MAC.String(),
		"client_id":    l.ClientID,
		"hostname":     l.Hostname,
		"subnet":       l.Subnet,
		"pool":         l.Pool,
		"state":        string(l.State),
		"start":        l.Start.Unix(),
		"expiry":       l.Expiry.Unix(),
		"remaining":    int64(time.Until(l.Expiry).Seconds()),
		"last_updated": l.LastUpdated.Unix(),
	})
}

// handleDeleteLease deletes (releases) a lease by IP.
func (s *Server) handleDeleteLease(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
		return
	}

	if err := s.leaseStore.Delete(ip); err != nil {
		JSONError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleListSubnets returns configured subnets.
func (s *Server) handleListSubnets(w http.ResponseWriter, r *http.Request) {
	type subnetResponse struct {
		Network      string   `json:"network"`
		Routers      []string `json:"routers,omitempty"`
		DNSServers   []string `json:"dns_servers,omitempty"`
		DomainName   string   `json:"domain_name,omitempty"`
		LeaseTime    string   `json:"lease_time,omitempty"`
		Pools        int      `json:"pools"`
		Reservations int      `json:"reservations"`
	}

	result := make([]subnetResponse, 0, len(s.cfg.Subnets))
	for _, sub := range s.cfg.Subnets {
		result = append(result, subnetResponse{
			Network:      sub.Network,
			Routers:      sub.Routers,
			DNSServers:   sub.DNSServers,
			DomainName:   sub.DomainName,
			LeaseTime:    sub.LeaseTime,
			Pools:        len(sub.Pools),
			Reservations: len(sub.Reservations),
		})
	}

	JSONResponse(w, http.StatusOK, result)
}

// handleListPools returns pool utilization statistics.
func (s *Server) handleListPools(w http.ResponseWriter, r *http.Request) {
	type poolResponse struct {
		Name        string  `json:"name"`
		Range       string  `json:"range"`
		Size        uint32  `json:"size"`
		Allocated   uint32  `json:"allocated"`
		Available   uint32  `json:"available"`
		Utilization float64 `json:"utilization_percent"`
	}

	result := make([]poolResponse, 0, len(s.pools))
	for _, p := range s.pools {
		result = append(result, poolResponse{
			Name:        p.String(),
			Range:       p.RangeString(),
			Size:        p.Size(),
			Allocated:   p.Allocated(),
			Available:   p.Available(),
			Utilization: p.Utilization(),
		})
	}

	JSONResponse(w, http.StatusOK, result)
}

// handleListConflicts returns active IP conflicts.
func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	if s.conflictTable == nil {
		JSONResponse(w, http.StatusOK, []interface{}{})
		return
	}

	active := s.conflictTable.AllActive()
	JSONResponse(w, http.StatusOK, active)
}

// handleGetConflict returns a single conflict record by IP.
func (s *Server) handleGetConflict(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
		return
	}

	if s.conflictTable == nil {
		JSONError(w, http.StatusNotFound, "not_found", "conflict detection not enabled")
		return
	}

	rec := s.conflictTable.Get(ip)
	if rec == nil {
		JSONError(w, http.StatusNotFound, "not_found", "no conflict record for this IP")
		return
	}

	JSONResponse(w, http.StatusOK, rec)
}

// handleClearConflict clears a conflict entry for an IP.
func (s *Server) handleClearConflict(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
		return
	}

	if s.conflictTable == nil {
		JSONError(w, http.StatusNotFound, "not_found", "conflict detection not enabled")
		return
	}

	if err := s.conflictTable.Clear(ip); err != nil {
		JSONError(w, http.StatusInternalServerError, "clear_failed", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// handleExcludeConflict permanently excludes an IP from allocation.
func (s *Server) handleExcludeConflict(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
		return
	}

	if s.conflictTable == nil {
		JSONError(w, http.StatusNotFound, "not_found", "conflict detection not enabled")
		return
	}

	// Add with permanent flag by adding enough times to exceed max count
	_, err := s.conflictTable.Add(ip, "admin_exclude", "", "")
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "exclude_failed", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]string{"status": "excluded"})
}

// handleConflictHistory returns recently resolved conflicts.
func (s *Server) handleConflictHistory(w http.ResponseWriter, r *http.Request) {
	if s.conflictTable == nil {
		JSONResponse(w, http.StatusOK, []interface{}{})
		return
	}

	resolved := s.conflictTable.AllResolved()
	JSONResponse(w, http.StatusOK, resolved)
}

// handleConflictStats returns conflict statistics.
func (s *Server) handleConflictStats(w http.ResponseWriter, r *http.Request) {
	if s.conflictTable == nil {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
		})
		return
	}

	active := s.conflictTable.AllActive()
	resolved := s.conflictTable.AllResolved()

	// Count by method
	byMethod := map[string]int{}
	bySubnet := map[string]int{}
	for _, r := range active {
		byMethod[r.DetectionMethod]++
		if r.Subnet != "" {
			bySubnet[r.Subnet]++
		}
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"enabled":         true,
		"active_count":    len(active),
		"resolved_count":  len(resolved),
		"permanent_count": s.conflictTable.PermanentCount(),
		"by_method":       byMethod,
		"by_subnet":       bySubnet,
	})
}

// handleGetConfig returns the current configuration (with secrets redacted).
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// Deep copy and redact secrets
	type redactedConfig struct {
		Server   interface{} `json:"server"`
		Subnets  interface{} `json:"subnets"`
		Defaults interface{} `json:"defaults"`
		DDNS     interface{} `json:"ddns"`
		HA       interface{} `json:"ha"`
		Hooks    interface{} `json:"hooks"`
		API      interface{} `json:"api"`
	}

	// Marshal/unmarshal to get a generic map we can redact
	raw, _ := json.Marshal(s.cfg)
	var cfgMap map[string]interface{}
	json.Unmarshal(raw, &cfgMap)

	// Redact sensitive fields
	redactSecrets(cfgMap)

	JSONResponse(w, http.StatusOK, cfgMap)
}

// handleGetStats returns server statistics.
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"leases": map[string]interface{}{
			"total": s.leaseStore.Count(),
		},
		"pools":     len(s.pools),
		"subnets":   len(s.cfg.Subnets),
		"timestamp": time.Now().Unix(),
	}

	if s.conflictTable != nil {
		stats["conflicts"] = map[string]interface{}{
			"active":    s.conflictTable.Count(),
			"permanent": s.conflictTable.PermanentCount(),
		}
	}

	JSONResponse(w, http.StatusOK, stats)
}

// redactSecrets recursively redacts sensitive values from a config map.
func redactSecrets(m map[string]interface{}) {
	sensitiveKeys := map[string]bool{
		"tsig_secret":   true,
		"tsig_key":      true,
		"api_key":       true,
		"auth_token":    true,
		"password_hash": true,
		"secret":        true,
		"key_file":      true,
	}

	for k, v := range m {
		if sensitiveKeys[k] {
			if s, ok := v.(string); ok && s != "" {
				m[k] = "***REDACTED***"
			}
		}
		if sub, ok := v.(map[string]interface{}); ok {
			redactSecrets(sub)
		}
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if sub, ok := item.(map[string]interface{}); ok {
					redactSecrets(sub)
				}
			}
		}
	}
}
