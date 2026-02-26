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
		"peer_address":   s.cfg.HA.PeerAddress,
		"listen_address": s.cfg.HA.ListenAddress,
	}

	if s.peer != nil {
		resp["peer_connected"] = s.peer.Connected()
	} else {
		resp["peer_connected"] = false
	}

	resp["is_standby"] = !s.fsm.IsActive()
	if primaryURL := s.primaryWebURL(); primaryURL != "" {
		resp["primary_url"] = primaryURL
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
