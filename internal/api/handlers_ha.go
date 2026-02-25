package api

import (
	"net/http"
	"time"
)

// handleHAStatus returns the current HA state.
func (s *Server) handleHAStatus(w http.ResponseWriter, r *http.Request) {
	if s.fsm == nil {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
		})
		return
	}

	resp := map[string]interface{}{
		"enabled":        true,
		"role":           s.fsm.Role(),
		"state":          string(s.fsm.State()),
		"is_active":      s.fsm.IsActive(),
		"last_heartbeat": s.fsm.LastHeartbeat().Format(time.RFC3339),
	}

	if s.cfg.HA.Enabled {
		resp["peer_address"] = s.cfg.HA.PeerAddress
		resp["listen_address"] = s.cfg.HA.ListenAddress
	}

	JSONResponse(w, http.StatusOK, resp)
}

// handleHAFailover triggers a manual failover.
func (s *Server) handleHAFailover(w http.ResponseWriter, r *http.Request) {
	if s.fsm == nil {
		JSONError(w, http.StatusBadRequest, "ha_disabled", "HA is not enabled")
		return
	}

	s.fsm.ClaimActive("API manual failover")
	s.logger.Warn("manual failover triggered via API")

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "failover initiated",
		"new_state": string(s.fsm.State()),
	})
}
