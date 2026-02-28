package api

import (
	"net/http"
	"net/url"
)

// handleFingerprintList returns all known device fingerprints.
// GET /api/v2/fingerprints
func (s *Server) handleFingerprintList(w http.ResponseWriter, r *http.Request) {
	if s.fpStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "fingerprint_disabled", "fingerprint store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.fpStore.All())
}

// handleFingerprintGet returns the fingerprint for a specific MAC.
// GET /api/v2/fingerprints/{mac}
func (s *Server) handleFingerprintGet(w http.ResponseWriter, r *http.Request) {
	if s.fpStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "fingerprint_disabled", "fingerprint store not available")
		return
	}
	mac, _ := url.PathUnescape(r.PathValue("mac"))
	info := s.fpStore.Get(mac)
	if info == nil {
		JSONError(w, http.StatusNotFound, "not_found", "no fingerprint for this MAC")
		return
	}
	JSONResponse(w, http.StatusOK, info)
}

// handleFingerprintStats returns fingerprint statistics.
// GET /api/v2/fingerprints/stats
func (s *Server) handleFingerprintStats(w http.ResponseWriter, r *http.Request) {
	if s.fpStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "fingerprint_disabled", "fingerprint store not available")
		return
	}

	all := s.fpStore.All()

	// Count by device type
	byType := make(map[string]int)
	byOS := make(map[string]int)
	for _, d := range all {
		byType[d.DeviceType]++
		if d.OS != "" {
			byOS[d.OS]++
		}
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"total_devices": len(all),
		"by_type":       byType,
		"by_os":         byOS,
		"has_api_key":   s.fpStore.HasFingerbank(),
	})
}
