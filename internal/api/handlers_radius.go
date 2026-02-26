package api

import (
	"encoding/json"
	"net/http"

	radiuspkg "github.com/athena-dhcpd/athena-dhcpd/internal/radius"
)

// handleRADIUSList returns all configured RADIUS subnets.
// GET /api/v2/radius
func (s *Server) handleRADIUSList(w http.ResponseWriter, r *http.Request) {
	if s.radiusClient == nil {
		JSONError(w, http.StatusServiceUnavailable, "radius_disabled", "RADIUS not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.radiusClient.ListSubnets())
}

// handleRADIUSGetSubnet returns RADIUS config for a specific subnet.
// GET /api/v2/radius/{subnet}
func (s *Server) handleRADIUSGetSubnet(w http.ResponseWriter, r *http.Request) {
	if s.radiusClient == nil {
		JSONError(w, http.StatusServiceUnavailable, "radius_disabled", "RADIUS not available")
		return
	}
	subnet := r.PathValue("subnet")
	cfg := s.radiusClient.GetSubnet(subnet)
	if cfg == nil {
		JSONError(w, http.StatusNotFound, "not_found", "no RADIUS config for this subnet")
		return
	}
	// Redact secret
	cfg.Server.Secret = "***"
	JSONResponse(w, http.StatusOK, cfg)
}

// handleRADIUSSetSubnet sets RADIUS config for a subnet.
// PUT /api/v2/radius/{subnet}
func (s *Server) handleRADIUSSetSubnet(w http.ResponseWriter, r *http.Request) {
	if s.radiusClient == nil {
		JSONError(w, http.StatusServiceUnavailable, "radius_disabled", "RADIUS not available")
		return
	}
	subnet := r.PathValue("subnet")
	var cfg radiuspkg.SubnetConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	s.radiusClient.SetSubnet(subnet, &cfg)
	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRADIUSDeleteSubnet removes RADIUS config for a subnet.
// DELETE /api/v2/radius/{subnet}
func (s *Server) handleRADIUSDeleteSubnet(w http.ResponseWriter, r *http.Request) {
	if s.radiusClient == nil {
		JSONError(w, http.StatusServiceUnavailable, "radius_disabled", "RADIUS not available")
		return
	}
	subnet := r.PathValue("subnet")
	s.radiusClient.RemoveSubnet(subnet)
	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRADIUSTest tests connectivity to a RADIUS server.
// POST /api/v2/radius/test
func (s *Server) handleRADIUSTest(w http.ResponseWriter, r *http.Request) {
	if s.radiusClient == nil {
		JSONError(w, http.StatusServiceUnavailable, "radius_disabled", "RADIUS not available")
		return
	}
	var req struct {
		Address  string `json:"address"`
		Secret   string `json:"secret"`
		Timeout  string `json:"timeout"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Address == "" || req.Secret == "" {
		JSONError(w, http.StatusBadRequest, "missing_fields", "address and secret are required")
		return
	}

	result := s.radiusClient.Test(r.Context(), &radiuspkg.ServerConfig{
		Address: req.Address,
		Secret:  req.Secret,
		Timeout: req.Timeout,
	}, req.Username, req.Password)

	JSONResponse(w, http.StatusOK, result)
}
