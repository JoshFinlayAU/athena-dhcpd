package api

import (
	"net/http"
)

// handleAnomalyWeather returns network weather for all subnets.
// GET /api/v2/anomaly/weather
func (s *Server) handleAnomalyWeather(w http.ResponseWriter, r *http.Request) {
	if s.anomalyDetector == nil {
		JSONError(w, http.StatusServiceUnavailable, "anomaly_disabled", "anomaly detection not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.anomalyDetector.Weather())
}
