package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

func TestAuthNoAuthConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auth := NewAuthMiddleware("", nil, logger)

	handler := auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("no auth configured should allow all, got %d", w.Code)
	}
}

func TestAuthBearerToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auth := NewAuthMiddleware("test-token", nil, logger)

	handler := auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Valid token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid token should allow, got %d", w.Code)
	}

	// Invalid token
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	w2 := httptest.NewRecorder()
	handler(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("invalid token should reject, got %d", w2.Code)
	}

	// No token
	req3 := httptest.NewRequest("GET", "/test", nil)
	w3 := httptest.NewRecorder()
	handler(w3, req3)

	if w3.Code != http.StatusUnauthorized {
		t.Errorf("no token should reject, got %d", w3.Code)
	}
}

func TestAuthQueryToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auth := NewAuthMiddleware("test-token", nil, logger)

	handler := auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?token=test-token", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("query token should allow, got %d", w.Code)
	}
}

func TestAuthBasicAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	users := []config.UserConfig{
		{Username: "admin", PasswordHash: string(hash), Role: "admin"},
		{Username: "viewer", PasswordHash: string(hash), Role: "viewer"},
	}

	auth := NewAuthMiddleware("", users, logger)

	// Test admin access
	adminHandler := auth.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.SetBasicAuth("admin", "password123")
	w := httptest.NewRecorder()
	adminHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("admin should be allowed, got %d", w.Code)
	}

	// Test viewer denied admin
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.SetBasicAuth("viewer", "password123")
	w2 := httptest.NewRecorder()
	adminHandler(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Errorf("viewer should be forbidden from admin endpoint, got %d", w2.Code)
	}

	// Test wrong password
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.SetBasicAuth("admin", "wrongpassword")
	w3 := httptest.NewRecorder()
	adminHandler(w3, req3)

	if w3.Code != http.StatusUnauthorized {
		t.Errorf("wrong password should be unauthorized, got %d", w3.Code)
	}
}
