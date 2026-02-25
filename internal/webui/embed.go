// Package webui provides the embedded web UI served via go:embed.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA.
// Non-API, non-metrics paths serve index.html for client-side routing.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webui: failed to sub dist: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Let API, metrics, and WebSocket paths pass through
		if strings.HasPrefix(path, "/api/") || path == "/metrics" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the static file
		if path != "/" {
			// Check if the file exists in the embedded FS
			cleanPath := strings.TrimPrefix(path, "/")
			if _, err := fs.Stat(sub, cleanPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for all other paths
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
