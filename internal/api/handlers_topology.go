package api

import (
	"encoding/json"
	"net/http"
)

// handleTopologyTree returns the full topology tree.
// GET /api/v2/topology
func (s *Server) handleTopologyTree(w http.ResponseWriter, r *http.Request) {
	if s.topoMap == nil {
		JSONError(w, http.StatusServiceUnavailable, "topology_disabled", "topology mapping not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.topoMap.Tree())
}

// handleTopologyStats returns topology summary statistics.
// GET /api/v2/topology/stats
func (s *Server) handleTopologyStats(w http.ResponseWriter, r *http.Request) {
	if s.topoMap == nil {
		JSONError(w, http.StatusServiceUnavailable, "topology_disabled", "topology mapping not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.topoMap.Stats())
}

// handleTopologySetLabel sets a label on a switch or port.
// POST /api/v2/topology/label
func (s *Server) handleTopologySetLabel(w http.ResponseWriter, r *http.Request) {
	if s.topoMap == nil {
		JSONError(w, http.StatusServiceUnavailable, "topology_disabled", "topology mapping not available")
		return
	}
	var req struct {
		SwitchID string `json:"switch_id"`
		PortID   string `json:"port_id"`
		Label    string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.topoMap.SetLabel(req.SwitchID, req.PortID, req.Label); err != nil {
		JSONError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
