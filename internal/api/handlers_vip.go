package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/athena-dhcpd/athena-dhcpd/internal/vip"
)

// handleGetVIPs returns the configured VIP entries.
func (s *Server) handleGetVIPs(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	raw := s.cfgStore.VIPs()
	if raw == nil {
		JSONResponse(w, http.StatusOK, []vip.Entry{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}

// handleSetVIPs replaces the VIP entries list.
func (s *Server) handleSetVIPs(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	var entries []vip.Entry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	// Validate entries
	for i, e := range entries {
		if e.IP == "" {
			JSONError(w, http.StatusBadRequest, "invalid_entry", fmt.Sprintf("entry %d: ip is required", i))
			return
		}
		if e.CIDR <= 0 || e.CIDR > 32 {
			JSONError(w, http.StatusBadRequest, "invalid_entry", fmt.Sprintf("entry %d: cidr must be 1-32", i))
			return
		}
		if e.Interface == "" {
			JSONError(w, http.StatusBadRequest, "invalid_entry", fmt.Sprintf("entry %d: interface is required", i))
			return
		}
	}

	data, err := json.Marshal(entries)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "marshal_error", err.Error())
		return
	}

	if err := s.cfgStore.SetVIPs(data); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Hot-reload the VIP group if it exists
	if s.vipGroup != nil {
		if err := s.vipGroup.Reload(entries); err != nil {
			s.logger.Error("failed to reload VIP group", "error", err)
		}
	}

	JSONResponse(w, http.StatusOK, entries)
}

// handleGetVIPStatus returns the runtime status of all VIPs.
func (s *Server) handleGetVIPStatus(w http.ResponseWriter, r *http.Request) {
	if s.vipGroup == nil {
		JSONResponse(w, http.StatusOK, vip.GroupStatus{})
		return
	}
	JSONResponse(w, http.StatusOK, s.vipGroup.Status())
}
