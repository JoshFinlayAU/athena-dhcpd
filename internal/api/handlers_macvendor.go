package api

import (
	"net/http"
	"net/url"
)

// handleMACVendorLookup returns the vendor for a specific MAC address.
// GET /api/v2/macvendor/{mac}
func (s *Server) handleMACVendorLookup(w http.ResponseWriter, r *http.Request) {
	if s.macVendorDB == nil {
		JSONError(w, http.StatusServiceUnavailable, "macvendor_disabled", "MAC vendor database not loaded")
		return
	}
	mac, _ := url.PathUnescape(r.PathValue("mac"))
	vendor := s.macVendorDB.Lookup(mac)
	if vendor == "" {
		JSONError(w, http.StatusNotFound, "not_found", "unknown vendor for this MAC")
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"mac": mac, "vendor": vendor})
}

// handleMACVendorStats returns MAC vendor database stats.
// GET /api/v2/macvendor/stats
func (s *Server) handleMACVendorStats(w http.ResponseWriter, r *http.Request) {
	if s.macVendorDB == nil {
		JSONError(w, http.StatusServiceUnavailable, "macvendor_disabled", "MAC vendor database not loaded")
		return
	}
	JSONResponse(w, http.StatusOK, map[string]int{"entries": s.macVendorDB.Count()})
}
