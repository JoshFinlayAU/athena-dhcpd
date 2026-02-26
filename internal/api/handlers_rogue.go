package api

import (
	"encoding/json"
	"net/http"
)

// handleRogueList returns all known rogue DHCP servers.
// GET /api/v2/rogue
func (s *Server) handleRogueList(w http.ResponseWriter, r *http.Request) {
	if s.rogueDetector == nil {
		JSONError(w, http.StatusServiceUnavailable, "rogue_disabled", "rogue detection not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.rogueDetector.All())
}

// handleRogueStats returns rogue detection statistics.
// GET /api/v2/rogue/stats
func (s *Server) handleRogueStats(w http.ResponseWriter, r *http.Request) {
	if s.rogueDetector == nil {
		JSONError(w, http.StatusServiceUnavailable, "rogue_disabled", "rogue detection not available")
		return
	}
	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"total":  s.rogueDetector.Count(),
		"active": s.rogueDetector.ActiveCount(),
	})
}

// handleRogueAcknowledge acknowledges a rogue server (suppresses repeated alerts).
// POST /api/v2/rogue/acknowledge
func (s *Server) handleRogueAcknowledge(w http.ResponseWriter, r *http.Request) {
	if s.rogueDetector == nil {
		JSONError(w, http.StatusServiceUnavailable, "rogue_disabled", "rogue detection not available")
		return
	}
	var req struct {
		ServerIP string `json:"server_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.rogueDetector.Acknowledge(req.ServerIP); err != nil {
		JSONError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}

// handleRogueRemove removes a rogue server entry.
// POST /api/v2/rogue/remove
func (s *Server) handleRogueRemove(w http.ResponseWriter, r *http.Request) {
	if s.rogueDetector == nil {
		JSONError(w, http.StatusServiceUnavailable, "rogue_disabled", "rogue detection not available")
		return
	}
	var req struct {
		ServerIP string `json:"server_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.rogueDetector.Remove(req.ServerIP); err != nil {
		JSONError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "removed"})
}
