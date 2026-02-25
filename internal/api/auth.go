package api

import (
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// AuthMiddleware handles Bearer token and session cookie authentication.
type AuthMiddleware struct {
	bearerToken string
	users       []config.UserConfig
	logger      *slog.Logger
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(bearerToken string, users []config.UserConfig, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		bearerToken: bearerToken,
		users:       users,
		logger:      logger,
	}
}

// RequireAuth wraps a handler to require authentication (any role).
func (a *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticate(r) {
			JSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next(w, r)
	}
}

// RequireAdmin wraps a handler to require admin role.
func (a *AuthMiddleware) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := a.authenticateAndGetRole(r)
		if role == "" {
			JSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if role != "admin" {
			JSONError(w, http.StatusForbidden, "forbidden", "admin role required")
			return
		}
		next(w, r)
	}
}

// authenticate checks if the request has valid credentials.
func (a *AuthMiddleware) authenticate(r *http.Request) bool {
	return a.authenticateAndGetRole(r) != ""
}

// authenticateAndGetRole returns the role if authenticated, empty string otherwise.
func (a *AuthMiddleware) authenticateAndGetRole(r *http.Request) string {
	// No auth configured — allow everything as admin
	if a.bearerToken == "" && len(a.users) == 0 {
		return "admin"
	}

	// Check Bearer token
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if a.bearerToken != "" && token == a.bearerToken {
				return "admin"
			}
		}

		// Check Basic auth against configured users
		if strings.HasPrefix(authHeader, "Basic ") {
			username, password, ok := r.BasicAuth()
			if ok {
				return a.checkUserCredentials(username, password)
			}
		}
	}

	// Check query parameter (for WebSocket connections)
	if token := r.URL.Query().Get("token"); token != "" {
		if a.bearerToken != "" && token == a.bearerToken {
			return "admin"
		}
	}

	return ""
}

// checkUserCredentials validates username/password against configured users.
func (a *AuthMiddleware) checkUserCredentials(username, password string) string {
	for _, user := range a.users {
		if user.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err == nil {
				return user.Role
			}
		}
	}
	return ""
}
