package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// session represents an authenticated user session.
type session struct {
	Username  string
	Role      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// AuthMiddleware handles Bearer token, session cookie, and basic authentication.
type AuthMiddleware struct {
	bearerToken  string
	users        []config.UserConfig
	cookieName   string
	cookieSecure bool
	sessionTTL   time.Duration
	logger       *slog.Logger

	mu       sync.RWMutex
	sessions map[string]*session // sessionID -> session
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(cfg config.APIConfig, logger *slog.Logger) *AuthMiddleware {
	ttl, err := time.ParseDuration(cfg.Session.Expiry)
	if err != nil {
		ttl = 24 * time.Hour
	}

	a := &AuthMiddleware{
		bearerToken:  cfg.Auth.AuthToken,
		users:        cfg.Auth.Users,
		cookieName:   cfg.Session.CookieName,
		cookieSecure: cfg.Session.Secure,
		sessionTTL:   ttl,
		logger:       logger,
		sessions:     make(map[string]*session),
	}

	// Background session cleanup every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			a.cleanExpired()
		}
	}()

	return a
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

	// Check session cookie first (web UI)
	if cookie, err := r.Cookie(a.cookieName); err == nil {
		if sess := a.getSession(cookie.Value); sess != nil {
			return sess.Role
		}
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

	// Check query parameter (for SSE/API connections)
	if token := r.URL.Query().Get("token"); token != "" {
		if a.bearerToken != "" && token == a.bearerToken {
			return "admin"
		}
	}

	return ""
}

// checkUserCredentials validates username/password against configured users.
func (a *AuthMiddleware) checkUserCredentials(username, password string) string {
	a.mu.RLock()
	users := make([]config.UserConfig, len(a.users))
	copy(users, a.users)
	a.mu.RUnlock()

	for _, user := range users {
		if user.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err == nil {
				return user.Role
			}
		}
	}
	return ""
}

// AuthRequired returns true if auth is configured (users or bearer token set).
func (a *AuthMiddleware) AuthRequired() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bearerToken != "" || len(a.users) > 0
}

// UpdateUsers replaces the user list at runtime (called when DB users change).
func (a *AuthMiddleware) UpdateUsers(users []config.UserConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users = users
}

// --- Session management ---

func (a *AuthMiddleware) createSession(username, role string) string {
	b := make([]byte, 32)
	rand.Read(b)
	id := hex.EncodeToString(b)

	a.mu.Lock()
	a.sessions[id] = &session{
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(a.sessionTTL),
	}
	a.mu.Unlock()

	return id
}

func (a *AuthMiddleware) getSession(id string) *session {
	a.mu.RLock()
	defer a.mu.RUnlock()
	sess, ok := a.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
}

func (a *AuthMiddleware) deleteSession(id string) {
	a.mu.Lock()
	delete(a.sessions, id)
	a.mu.Unlock()
}

func (a *AuthMiddleware) cleanExpired() {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for id, sess := range a.sessions {
		if now.After(sess.ExpiresAt) {
			delete(a.sessions, id)
		}
	}
}

// --- Login/Logout/Me handlers ---

// handleLogin authenticates a user and creates a session cookie.
func (a *AuthMiddleware) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	role := a.checkUserCredentials(body.Username, body.Password)
	if role == "" {
		a.logger.Warn("failed login attempt", "username", body.Username)
		JSONError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}

	sessionID := a.createSession(body.Username, role)

	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.sessionTTL.Seconds()),
	})

	a.logger.Info("user logged in", "username", body.Username, "role", role)
	JSONResponse(w, http.StatusOK, map[string]string{
		"username": body.Username,
		"role":     role,
	})
}

// handleLogout destroys the session and clears the cookie.
func (a *AuthMiddleware) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.cookieName); err == nil {
		a.deleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	JSONResponse(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// handleMe returns the current authenticated user info.
func (a *AuthMiddleware) handleMe(w http.ResponseWriter, r *http.Request) {
	// No auth configured — return anonymous admin
	if !a.AuthRequired() {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"username":      "admin",
			"role":          "admin",
			"auth_required": false,
		})
		return
	}

	// Check session cookie
	if cookie, err := r.Cookie(a.cookieName); err == nil {
		if sess := a.getSession(cookie.Value); sess != nil {
			JSONResponse(w, http.StatusOK, map[string]interface{}{
				"authenticated": true,
				"username":      sess.Username,
				"role":          sess.Role,
				"auth_required": true,
			})
			return
		}
	}

	// Check bearer token
	role := a.authenticateAndGetRole(r)
	if role != "" {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"username":      "api",
			"role":          role,
			"auth_required": true,
		})
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"authenticated": false,
		"auth_required": true,
	})
}
