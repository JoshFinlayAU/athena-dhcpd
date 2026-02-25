package api

import (
	"net/http"
	"strings"
)

// handleSPA serves the embedded React SPA.
// All non-API, non-metrics paths serve index.html for client-side routing.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	// API and metrics paths are handled by their own routes
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/metrics") {
		http.NotFound(w, r)
		return
	}

	// For now, serve a placeholder until the React SPA is built
	// In production, this will use go:embed to serve web/dist/
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>athena-dhcpd</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; 
         background: #0f172a; color: #e2e8f0; display: flex; justify-content: center; 
         align-items: center; min-height: 100vh; margin: 0; }
  .container { text-align: center; }
  h1 { font-size: 2rem; color: #38bdf8; }
  p { color: #94a3b8; }
  code { background: #1e293b; padding: 2px 8px; border-radius: 4px; }
</style>
</head>
<body>
<div class="container">
  <h1>athena-dhcpd</h1>
  <p>Web UI placeholder — build the React SPA with <code>make build-web</code></p>
  <p>API available at <code>/api/v1/</code></p>
</div>
</body>
</html>`))
}
