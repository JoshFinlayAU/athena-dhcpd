package api

import (
	"net/http"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// handleListEvents returns recent events (from config, not persisted yet).
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	// For now, return an empty list — event persistence (BoltDB event_log bucket)
	// will be wired in later. The WebSocket stream provides live events.
	JSONResponse(w, http.StatusOK, []interface{}{})
}

// handleListHooks returns configured hook status.
func (s *Server) handleListHooks(w http.ResponseWriter, r *http.Request) {
	type hookInfo struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Events  []string `json:"events"`
		Subnets []string `json:"subnets,omitempty"`
		Enabled bool     `json:"enabled"`
	}

	var hooks []hookInfo

	for _, sh := range s.cfg.Hooks.Scripts {
		hooks = append(hooks, hookInfo{
			Name:    sh.Name,
			Type:    "script",
			Events:  sh.Events,
			Subnets: sh.Subnets,
			Enabled: true,
		})
	}

	for _, wh := range s.cfg.Hooks.Webhooks {
		hooks = append(hooks, hookInfo{
			Name:    wh.Name,
			Type:    "webhook",
			Events:  wh.Events,
			Enabled: true,
		})
	}

	JSONResponse(w, http.StatusOK, hooks)
}

// handleTestHook fires a test event through the event bus.
func (s *Server) handleTestHook(w http.ResponseWriter, r *http.Request) {
	evt := events.Event{
		Type:      events.EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			Hostname: "test-hook-event",
			Subnet:   "0.0.0.0/0",
		},
		Server: &events.ServerData{
			NodeID: "test",
		},
		Reason: "API test hook trigger",
	}

	s.bus.Publish(evt)

	JSONResponse(w, http.StatusOK, map[string]string{
		"status": "test event published",
	})
}
