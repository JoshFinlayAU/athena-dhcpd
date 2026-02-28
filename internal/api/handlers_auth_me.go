package api

import (
	"net/http"
)

// handleAuthMe returns the current authenticated user info.
// Moved from AuthMiddleware to Server so it can check the dbconfig store for users.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	// Check if ANY users are configured (TOML + DB)
	hasUsers := s.auth.AuthRequired()

	// If no users at all, tell the frontend to create an admin account
	if !hasUsers {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated":    false,
			"auth_required":    true,
			"needs_user_setup": true,
		})
		return
	}

	// Auth is configured — check if the request is authenticated
	// Check session cookie
	if cookie, err := r.Cookie(s.auth.cookieName); err == nil {
		if sess := s.auth.getSession(cookie.Value); sess != nil {
			JSONResponse(w, http.StatusOK, map[string]interface{}{
				"authenticated": true,
				"username":      sess.Username,
				"role":          sess.Role,
				"auth_required": true,
			})
			return
		}
	}

	// Check bearer token / basic auth
	role := s.auth.authenticateAndGetRole(r)
	if role != "" {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"username":      "api",
			"role":          role,
			"auth_required": true,
		})
		return
	}

	// Not authenticated
	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"authenticated": false,
		"auth_required": true,
	})
}
