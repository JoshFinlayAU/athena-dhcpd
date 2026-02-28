package api

import (
	"encoding/json"
	"net/http"

	"github.com/athena-dhcpd/athena-dhcpd/internal/portauto"
)

// handlePortAutoGetRules returns all port automation rules.
// GET /api/v2/portauto/rules
func (s *Server) handlePortAutoGetRules(w http.ResponseWriter, r *http.Request) {
	if s.portAutoEngine == nil {
		JSONError(w, http.StatusServiceUnavailable, "portauto_disabled", "port automation not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.portAutoEngine.GetRules())
}

// handlePortAutoSetRules replaces all port automation rules.
// PUT /api/v2/portauto/rules
func (s *Server) handlePortAutoSetRules(w http.ResponseWriter, r *http.Request) {
	if s.portAutoEngine == nil {
		JSONError(w, http.StatusServiceUnavailable, "portauto_disabled", "port automation not available")
		return
	}
	var rules []portauto.Rule
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.portAutoEngine.SetRules(rules); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	// Persist to database
	if s.cfgStore != nil {
		data, _ := json.Marshal(rules)
		if err := s.cfgStore.SetPortAutoRules(data); err != nil {
			s.logger.Warn("failed to persist portauto rules", "error", err)
		}
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePortAutoTest tests rules against a sample lease context.
// POST /api/v2/portauto/test
func (s *Server) handlePortAutoTest(w http.ResponseWriter, r *http.Request) {
	if s.portAutoEngine == nil {
		JSONError(w, http.StatusServiceUnavailable, "portauto_disabled", "port automation not available")
		return
	}
	var ctx portauto.LeaseContext
	if err := json.NewDecoder(r.Body).Decode(&ctx); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	results := s.portAutoEngine.Evaluate(ctx)
	JSONResponse(w, http.StatusOK, results)
}
