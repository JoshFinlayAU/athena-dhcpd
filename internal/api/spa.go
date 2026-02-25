package api

import (
	"net/http"

	"github.com/athena-dhcpd/athena-dhcpd/internal/webui"
)

// spaHandler is the embedded web UI handler, initialised once.
var spaHandler = webui.Handler()

// handleSPA serves the embedded React SPA.
// All non-API, non-metrics paths serve index.html for client-side routing.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	spaHandler.ServeHTTP(w, r)
}
