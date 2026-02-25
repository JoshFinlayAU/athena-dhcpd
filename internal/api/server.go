// Package api provides the HTTP API server, router, auth, and SSE event streaming.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dnsproxy"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/ha"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
)

// Server is the HTTP API server for athena-dhcpd.
type Server struct {
	cfg           *config.Config
	configPath    string
	leaseStore    *lease.Store
	leaseManager  *lease.Manager
	conflictTable *conflict.Table
	pools         []*pool.Pool
	bus           *events.Bus
	fsm           *ha.FSM
	peer          *ha.Peer
	dns           *dnsproxy.Server
	logger        *slog.Logger
	httpServer    *http.Server
	auth          *AuthMiddleware
	sseHub        *SSEHub
}

// NewServer creates a new API server.
func NewServer(
	cfg *config.Config,
	store *lease.Store,
	mgr *lease.Manager,
	ct *conflict.Table,
	pools []*pool.Pool,
	bus *events.Bus,
	logger *slog.Logger,
	opts ...ServerOption,
) *Server {
	s := &Server{
		cfg:           cfg,
		leaseStore:    store,
		leaseManager:  mgr,
		conflictTable: ct,
		pools:         pools,
		bus:           bus,
		logger:        logger,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.auth = NewAuthMiddleware(cfg.API, logger)
	s.sseHub = NewSSEHub(bus, logger)

	return s
}

// ServerOption configures optional Server fields.
type ServerOption func(*Server)

// WithConfigPath sets the config file path for write-back support.
func WithConfigPath(path string) ServerOption {
	return func(s *Server) { s.configPath = path }
}

// WithFSM sets the HA failover state machine.
func WithFSM(fsm *ha.FSM) ServerOption {
	return func(s *Server) { s.fsm = fsm }
}

// WithPeer sets the HA peer connection.
func WithPeer(peer *ha.Peer) ServerOption {
	return func(s *Server) { s.peer = peer }
}

// WithDNSProxy sets the built-in DNS proxy server.
func WithDNSProxy(dns *dnsproxy.Server) ServerOption {
	return func(s *Server) { s.dns = dns }
}

// Start begins serving the HTTP API.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Wrap with metrics middleware
	handler := newMetricsMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:        s.cfg.API.Listen,
		Handler:     handler,
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
		// No WriteTimeout — SSE streams need to stay open
	}

	// Start SSE hub
	go s.sseHub.Run()

	s.logger.Info("API server starting", "listen", s.cfg.API.Listen)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("API server: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the API server.
func (s *Server) Stop(ctx context.Context) error {
	s.sseHub.Stop()
	return s.httpServer.Shutdown(ctx)
}

// registerRoutes sets up all API endpoints.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Prometheus metrics (no auth)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Health check (no auth)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// Auth (no auth required — these handle their own auth)
	mux.HandleFunc("POST /api/v1/auth/login", s.auth.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.auth.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", s.auth.handleMe)

	// Leases
	mux.HandleFunc("GET /api/v1/leases", s.auth.RequireAuth(s.handleListLeases))
	mux.HandleFunc("GET /api/v1/leases/export", s.auth.RequireAuth(s.handleExportLeases))
	mux.HandleFunc("GET /api/v1/leases/{ip}", s.auth.RequireAuth(s.handleGetLease))
	mux.HandleFunc("DELETE /api/v1/leases/{ip}", s.auth.RequireAdmin(s.handleDeleteLease))

	// Reservations
	mux.HandleFunc("GET /api/v1/reservations", s.auth.RequireAuth(s.handleListReservations))
	mux.HandleFunc("POST /api/v1/reservations", s.auth.RequireAdmin(s.handleCreateReservation))
	mux.HandleFunc("PUT /api/v1/reservations/{id}", s.auth.RequireAdmin(s.handleUpdateReservation))
	mux.HandleFunc("DELETE /api/v1/reservations/{id}", s.auth.RequireAdmin(s.handleDeleteReservation))
	mux.HandleFunc("POST /api/v1/reservations/import", s.auth.RequireAdmin(s.handleImportReservations))
	mux.HandleFunc("GET /api/v1/reservations/export", s.auth.RequireAuth(s.handleExportReservations))

	// Subnets & Pools
	mux.HandleFunc("GET /api/v1/subnets", s.auth.RequireAuth(s.handleListSubnets))
	mux.HandleFunc("GET /api/v1/pools", s.auth.RequireAuth(s.handleListPools))

	// Conflicts
	mux.HandleFunc("GET /api/v1/conflicts", s.auth.RequireAuth(s.handleListConflicts))
	mux.HandleFunc("GET /api/v1/conflicts/history", s.auth.RequireAuth(s.handleConflictHistory))
	mux.HandleFunc("GET /api/v1/conflicts/stats", s.auth.RequireAuth(s.handleConflictStats))
	mux.HandleFunc("GET /api/v1/conflicts/{ip}", s.auth.RequireAuth(s.handleGetConflict))
	mux.HandleFunc("DELETE /api/v1/conflicts/{ip}", s.auth.RequireAdmin(s.handleClearConflict))
	mux.HandleFunc("POST /api/v1/conflicts/{ip}/exclude", s.auth.RequireAdmin(s.handleExcludeConflict))

	// Config
	mux.HandleFunc("GET /api/v1/config", s.auth.RequireAuth(s.handleGetConfig))
	mux.HandleFunc("GET /api/v1/config/raw", s.auth.RequireAuth(s.handleGetConfigRaw))
	mux.HandleFunc("PUT /api/v1/config", s.auth.RequireAdmin(s.handleUpdateConfig))
	mux.HandleFunc("POST /api/v1/config/validate", s.auth.RequireAuth(s.handleValidateConfig))
	mux.HandleFunc("GET /api/v1/config/backups", s.auth.RequireAuth(s.handleListConfigBackups))
	mux.HandleFunc("GET /api/v1/config/backups/{timestamp}", s.auth.RequireAuth(s.handleGetConfigBackup))

	// Events & Hooks
	mux.HandleFunc("GET /api/v1/events", s.auth.RequireAuth(s.handleListEvents))
	mux.HandleFunc("GET /api/v1/events/stream", s.auth.RequireAuth(s.handleSSE))
	mux.HandleFunc("GET /api/v1/hooks", s.auth.RequireAuth(s.handleListHooks))
	mux.HandleFunc("POST /api/v1/hooks/test", s.auth.RequireAdmin(s.handleTestHook))

	// HA
	mux.HandleFunc("GET /api/v1/ha/status", s.auth.RequireAuth(s.handleHAStatus))
	mux.HandleFunc("POST /api/v1/ha/failover", s.auth.RequireAdmin(s.handleHAFailover))

	// DNS proxy
	mux.HandleFunc("GET /api/v1/dns/stats", s.auth.RequireAuth(s.handleDNSStats))
	mux.HandleFunc("POST /api/v1/dns/cache/flush", s.auth.RequireAdmin(s.handleDNSFlushCache))
	mux.HandleFunc("GET /api/v1/dns/records", s.auth.RequireAuth(s.handleDNSListRecords))
	mux.HandleFunc("GET /api/v1/dns/lists", s.auth.RequireAuth(s.handleDNSListStatus))
	mux.HandleFunc("POST /api/v1/dns/lists/refresh", s.auth.RequireAdmin(s.handleDNSListRefresh))
	mux.HandleFunc("POST /api/v1/dns/lists/test", s.auth.RequireAuth(s.handleDNSListTest))

	// Stats
	mux.HandleFunc("GET /api/v1/stats", s.auth.RequireAuth(s.handleGetStats))

	// SPA fallback — serve index.html for all non-API paths
	if s.cfg.API.WebUI {
		mux.HandleFunc("/", s.handleSPA)
	}
}

// JSONResponse writes a JSON response with the given status code.
func JSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// JSONError writes a JSON error response.
func JSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
}
