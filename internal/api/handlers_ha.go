package api

import (
	"net/http"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/vrrp"
)

// handleHAStatus returns the current HA state.
func (s *Server) handleHAStatus(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"enabled": false,
	}

	if s.fsm != nil {
		resp["enabled"] = true
		resp["role"] = s.fsm.Role()
		resp["state"] = string(s.fsm.State())
		resp["is_active"] = s.fsm.IsActive()
		resp["last_heartbeat"] = s.fsm.LastHeartbeat().Format(time.RFC3339)
		resp["peer_address"] = s.cfg.HA.PeerAddress
		resp["listen_address"] = s.cfg.HA.ListenAddress

		if s.peer != nil {
			resp["peer_connected"] = s.peer.Connected()
			if errMsg, errAt := s.peer.LastConnError(); errMsg != "" {
				resp["last_error"] = errMsg
				resp["last_error_at"] = errAt.Format(time.RFC3339)
			}
		} else {
			resp["peer_connected"] = false
		}

		resp["is_standby"] = !s.fsm.IsActive()
		if primaryURL := s.primaryWebURL(); primaryURL != "" {
			resp["primary_url"] = primaryURL
		}
	}

	// VRRP/keepalived auto-detection (independent of HA FSM, no config needed)
	if v := vrrp.Detect(); v != nil {
		resp["vrrp"] = v
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
