package api

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// handleCreateUser creates a new user or updates an existing one.
// POST /api/v2/auth/users
// This endpoint is special: if NO users exist yet, it does NOT require auth
// (allows initial admin account creation). Otherwise, it requires admin role.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	// If users already exist, require admin auth
	if s.auth.AuthRequired() {
		role := s.auth.authenticateAndGetRole(r)
		if role == "" {
			JSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if role != "admin" {
			JSONError(w, http.StatusForbidden, "forbidden", "admin role required")
			return
		}
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if body.Username == "" || body.Password == "" {
		JSONError(w, http.StatusBadRequest, "bad_request", "username and password are required")
		return
	}
	if body.Role == "" {
		body.Role = "admin"
	}
	if body.Role != "admin" && body.Role != "viewer" {
		JSONError(w, http.StatusBadRequest, "bad_request", "role must be 'admin' or 'viewer'")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "hash_error", "failed to hash password")
		return
	}

	u := config.UserConfig{
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         body.Role,
	}

	if err := s.cfgStore.PutUser(u); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Hot-reload the auth middleware with updated user list
	s.auth.UpdateUsers(s.cfgStore.Users())

	s.logger.Info("user created", "username", body.Username, "role", body.Role)
	JSONResponse(w, http.StatusOK, map[string]string{
		"username": body.Username,
		"role":     body.Role,
		"status":   "created",
	})
}

// handleListUsers returns all configured users (without password hashes).
// GET /api/v2/auth/users
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	users := s.cfgStore.Users()
	result := make([]map[string]string, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]string{
			"username": u.Username,
			"role":     u.Role,
		})
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"users": result,
	})
}

// handleDeleteUser removes a user.
// DELETE /api/v2/auth/users/{username}
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	username := r.PathValue("username")
	if username == "" {
		JSONError(w, http.StatusBadRequest, "bad_request", "username is required")
		return
	}

	if err := s.cfgStore.DeleteUser(username); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	s.auth.UpdateUsers(s.cfgStore.Users())

	s.logger.Info("user deleted", "username", username)
	JSONResponse(w, http.StatusOK, map[string]string{
		"status": "deleted",
	})
}
