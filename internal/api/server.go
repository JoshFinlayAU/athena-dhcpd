// Package api provides the HTTP API server, router, auth, and SSE event streaming.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/athena-dhcpd/athena-dhcpd/internal/anomaly"
	"github.com/athena-dhcpd/athena-dhcpd/internal/audit"
	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dbconfig"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dnsproxy"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/fingerprint"
	"github.com/athena-dhcpd/athena-dhcpd/internal/ha"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/macvendor"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
	"github.com/athena-dhcpd/athena-dhcpd/internal/portauto"
	radiuspkg "github.com/athena-dhcpd/athena-dhcpd/internal/radius"
	"github.com/athena-dhcpd/athena-dhcpd/internal/rogue"
	"github.com/athena-dhcpd/athena-dhcpd/internal/topology"
)

// Server is the HTTP API server for athena-dhcpd.
type Server struct {
	cfg             *config.Config
	configPath      string
	leaseStore      *lease.Store
	leaseManager    *lease.Manager
	conflictTable   *conflict.Table
	pools           []*pool.Pool
	bus             *events.Bus
	fsm             *ha.FSM
	peer            *ha.Peer
	dns             *dnsproxy.Server
	auditLog        *audit.Log
	fpStore         *fingerprint.Store
	rogueDetector   *rogue.Detector
	topoMap         *topology.Map
	anomalyDetector *anomaly.Detector
	macVendorDB     *macvendor.DB
	radiusClient    *radiuspkg.Client
	portAutoEngine  *portauto.Engine
	logger          *slog.Logger
	httpServer      *http.Server
	auth            *AuthMiddleware
	sseHub          *SSEHub
	cfgStore        *dbconfig.Store
	startTime       time.Time
	version         string
	setupMode       bool
	onSetupComplete func()
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
		startTime:     time.Now(),
		version:       "dev",
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

// WithVersion sets the server version string.
func WithVersion(v string) ServerOption {
	return func(s *Server) { s.version = v }
}

// WithConfigStore sets the database-backed config store.
func WithConfigStore(cs *dbconfig.Store) ServerOption {
	return func(s *Server) { s.cfgStore = cs }
}

// WithAuditLog sets the lease audit log.
func WithAuditLog(al *audit.Log) ServerOption {
	return func(s *Server) { s.auditLog = al }
}

// WithFingerprintStore sets the device fingerprint store.
func WithFingerprintStore(fs *fingerprint.Store) ServerOption {
	return func(s *Server) { s.fpStore = fs }
}

// WithRogueDetector sets the rogue DHCP server detector.
func WithRogueDetector(rd *rogue.Detector) ServerOption {
	return func(s *Server) { s.rogueDetector = rd }
}

// WithTopologyMap sets the Option 82 topology map.
func WithTopologyMap(tm *topology.Map) ServerOption {
	return func(s *Server) { s.topoMap = tm }
}

// WithAnomalyDetector sets the anomaly detector.
func WithAnomalyDetector(ad *anomaly.Detector) ServerOption {
	return func(s *Server) { s.anomalyDetector = ad }
}

// WithMACVendorDB sets the MAC vendor lookup database.
func WithMACVendorDB(db *macvendor.DB) ServerOption {
	return func(s *Server) { s.macVendorDB = db }
}

// WithRADIUSClient sets the RADIUS authentication client.
func WithRADIUSClient(rc *radiuspkg.Client) ServerOption {
	return func(s *Server) { s.radiusClient = rc }
}

// WithPortAutoEngine sets the port automation engine.
func WithPortAutoEngine(pa *portauto.Engine) ServerOption {
	return func(s *Server) { s.portAutoEngine = pa }
}

// WithSetupMode marks this server as running in setup wizard mode.
func WithSetupMode(cb func()) ServerOption {
	return func(s *Server) {
		s.setupMode = true
		s.onSetupComplete = cb
	}
}

// Listen binds the API server to its configured address and prepares routes.
// Call this synchronously to catch port conflicts before starting background serve.
func (s *Server) Listen() (net.Listener, error) {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := newMetricsMiddleware(mux)

	s.httpServer = &http.Server{
		Handler:     handler,
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
		// No WriteTimeout — SSE streams need to stay open
	}

	ln, err := net.Listen("tcp", s.cfg.API.Listen)
	if err != nil {
		return nil, fmt.Errorf("binding API server to %s: %w", s.cfg.API.Listen, err)
	}

	go s.sseHub.Run()

	s.logger.Info("API server listening", "address", ln.Addr().String())
	return ln, nil
}

// Serve accepts connections on the listener. Blocks until shutdown.
func (s *Server) Serve(ln net.Listener) error {
	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("API server: %w", err)
	}
	return nil
}

// Start is a convenience that calls Listen + Serve. Blocks until shutdown.
func (s *Server) Start() error {
	ln, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Stop gracefully shuts down the API server.
func (s *Server) Stop(ctx context.Context) error {
	s.sseHub.Stop()
	// Stop HA peer if one was started during the setup wizard
	if s.setupMode && s.peer != nil {
		s.peer.Stop()
		s.peer = nil
		s.fsm = nil
	}
	return s.httpServer.Shutdown(ctx)
}

// registerRoutes sets up all API endpoints.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Prometheus metrics (no auth)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Health check (no auth)
	mux.HandleFunc("GET /api/v2/health", s.handleHealth)

	// Auth (no auth required — these handle their own auth)
	mux.HandleFunc("POST /api/v2/auth/login", s.auth.handleLogin)
	mux.HandleFunc("POST /api/v2/auth/logout", s.auth.handleLogout)
	mux.HandleFunc("GET /api/v2/auth/me", s.handleAuthMe)

	// User management
	mux.HandleFunc("POST /api/v2/auth/users", s.handleCreateUser)
	mux.HandleFunc("GET /api/v2/auth/users", s.auth.RequireAdmin(s.handleListUsers))
	mux.HandleFunc("DELETE /api/v2/auth/users/{username}", s.auth.RequireAdmin(s.handleDeleteUser))

	// Leases
	mux.HandleFunc("GET /api/v2/leases", s.auth.RequireAuth(s.handleListLeases))
	mux.HandleFunc("GET /api/v2/leases/export", s.auth.RequireAuth(s.handleExportLeases))
	mux.HandleFunc("GET /api/v2/leases/{ip}", s.auth.RequireAuth(s.handleGetLease))
	mux.HandleFunc("DELETE /api/v2/leases/{ip}", s.auth.RequireAdmin(s.handleDeleteLease))

	// Reservations (flat/global view — reads from all subnets)
	mux.HandleFunc("GET /api/v2/reservations", s.auth.RequireAuth(s.handleListReservations))
	mux.HandleFunc("POST /api/v2/reservations", s.auth.RequireAdmin(s.handleCreateReservation))
	mux.HandleFunc("PUT /api/v2/reservations/{id}", s.auth.RequireAdmin(s.handleUpdateReservation))
	mux.HandleFunc("DELETE /api/v2/reservations/{id}", s.auth.RequireAdmin(s.handleDeleteReservation))
	mux.HandleFunc("POST /api/v2/reservations/import", s.auth.RequireAdmin(s.handleImportReservations))
	mux.HandleFunc("GET /api/v2/reservations/export", s.auth.RequireAuth(s.handleExportReservations))

	// Subnets & Pools (read-only runtime view)
	mux.HandleFunc("GET /api/v2/subnets", s.auth.RequireAuth(s.handleListSubnets))
	mux.HandleFunc("GET /api/v2/pools", s.auth.RequireAuth(s.handleListPools))

	// Conflicts
	mux.HandleFunc("GET /api/v2/conflicts", s.auth.RequireAuth(s.handleListConflicts))
	mux.HandleFunc("GET /api/v2/conflicts/history", s.auth.RequireAuth(s.handleConflictHistory))
	mux.HandleFunc("GET /api/v2/conflicts/stats", s.auth.RequireAuth(s.handleConflictStats))
	mux.HandleFunc("GET /api/v2/conflicts/{ip}", s.auth.RequireAuth(s.handleGetConflict))
	mux.HandleFunc("DELETE /api/v2/conflicts/{ip}", s.auth.RequireAdmin(s.handleClearConflict))
	mux.HandleFunc("POST /api/v2/conflicts/{ip}/exclude", s.auth.RequireAdmin(s.handleExcludeConflict))

	// Config (DB-backed CRUD)
	mux.HandleFunc("GET /api/v2/config/subnets", s.auth.RequireAuth(s.handleV2ListSubnets))
	mux.HandleFunc("POST /api/v2/config/subnets", s.auth.RequireAdmin(s.standbyGuard(s.handleV2CreateSubnet)))
	mux.HandleFunc("PUT /api/v2/config/subnets/{network}", s.auth.RequireAdmin(s.standbyGuard(s.handleV2UpdateSubnet)))
	mux.HandleFunc("DELETE /api/v2/config/subnets/{network}", s.auth.RequireAdmin(s.standbyGuard(s.handleV2DeleteSubnet)))
	mux.HandleFunc("GET /api/v2/config/subnets/{network}/reservations", s.auth.RequireAuth(s.handleV2ListReservations))
	mux.HandleFunc("POST /api/v2/config/subnets/{network}/reservations", s.auth.RequireAdmin(s.standbyGuard(s.handleV2CreateReservation)))
	mux.HandleFunc("DELETE /api/v2/config/subnets/{network}/reservations/{mac}", s.auth.RequireAdmin(s.standbyGuard(s.handleV2DeleteReservation)))
	mux.HandleFunc("POST /api/v2/config/subnets/{network}/reservations/import", s.auth.RequireAdmin(s.standbyGuard(s.handleV2ImportReservations)))
	mux.HandleFunc("GET /api/v2/config/defaults", s.auth.RequireAuth(s.handleV2GetDefaults))
	mux.HandleFunc("PUT /api/v2/config/defaults", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetDefaults)))
	mux.HandleFunc("GET /api/v2/config/conflict", s.auth.RequireAuth(s.handleV2GetConflict))
	mux.HandleFunc("PUT /api/v2/config/conflict", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetConflict)))
	mux.HandleFunc("GET /api/v2/config/ha", s.auth.RequireAuth(s.handleV2GetHA))
	mux.HandleFunc("PUT /api/v2/config/ha", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetHA)))
	mux.HandleFunc("GET /api/v2/config/hooks", s.auth.RequireAuth(s.handleV2GetHooks))
	mux.HandleFunc("PUT /api/v2/config/hooks", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetHooks)))
	mux.HandleFunc("GET /api/v2/config/ddns", s.auth.RequireAuth(s.handleV2GetDDNS))
	mux.HandleFunc("PUT /api/v2/config/ddns", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetDDNS)))
	mux.HandleFunc("GET /api/v2/config/dns", s.auth.RequireAuth(s.handleV2GetDNS))
	mux.HandleFunc("PUT /api/v2/config/dns", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetDNS)))
	mux.HandleFunc("GET /api/v2/config/hostname-sanitisation", s.auth.RequireAuth(s.handleV2GetHostnameSanitisation))
	mux.HandleFunc("PUT /api/v2/config/hostname-sanitisation", s.auth.RequireAdmin(s.standbyGuard(s.handleV2SetHostnameSanitisation)))
	mux.HandleFunc("POST /api/v2/config/import", s.auth.RequireAdmin(s.standbyGuard(s.handleV2ImportTOML)))
	mux.HandleFunc("GET /api/v2/config/raw", s.auth.RequireAuth(s.handleGetConfigRaw))
	mux.HandleFunc("POST /api/v2/config/validate", s.auth.RequireAuth(s.handleValidateConfig))

	// Events & Hooks
	mux.HandleFunc("GET /api/v2/events", s.auth.RequireAuth(s.handleListEvents))
	mux.HandleFunc("GET /api/v2/events/stream", s.auth.RequireAuth(s.handleSSE))
	mux.HandleFunc("GET /api/v2/hooks", s.auth.RequireAuth(s.handleListHooks))
	mux.HandleFunc("POST /api/v2/hooks/test", s.auth.RequireAdmin(s.handleTestHook))

	// Audit log
	mux.HandleFunc("GET /api/v2/audit", s.auth.RequireAuth(s.handleAuditQuery))
	mux.HandleFunc("GET /api/v2/audit/export", s.auth.RequireAuth(s.handleAuditExportCSV))
	mux.HandleFunc("GET /api/v2/audit/stats", s.auth.RequireAuth(s.handleAuditStats))

	// Device fingerprints
	mux.HandleFunc("GET /api/v2/fingerprints", s.auth.RequireAuth(s.handleFingerprintList))
	mux.HandleFunc("GET /api/v2/fingerprints/stats", s.auth.RequireAuth(s.handleFingerprintStats))
	mux.HandleFunc("GET /api/v2/fingerprints/{mac}", s.auth.RequireAuth(s.handleFingerprintGet))

	// Rogue DHCP server detection
	mux.HandleFunc("GET /api/v2/rogue", s.auth.RequireAuth(s.handleRogueList))
	mux.HandleFunc("GET /api/v2/rogue/stats", s.auth.RequireAuth(s.handleRogueStats))
	mux.HandleFunc("POST /api/v2/rogue/acknowledge", s.auth.RequireAdmin(s.handleRogueAcknowledge))
	mux.HandleFunc("POST /api/v2/rogue/remove", s.auth.RequireAdmin(s.handleRogueRemove))
	mux.HandleFunc("POST /api/v2/rogue/scan", s.auth.RequireAdmin(s.handleRogueScan))

	// Topology
	mux.HandleFunc("GET /api/v2/topology", s.auth.RequireAuth(s.handleTopologyTree))
	mux.HandleFunc("GET /api/v2/topology/stats", s.auth.RequireAuth(s.handleTopologyStats))
	mux.HandleFunc("POST /api/v2/topology/label", s.auth.RequireAdmin(s.handleTopologySetLabel))

	// Anomaly detection
	mux.HandleFunc("GET /api/v2/anomaly/weather", s.auth.RequireAuth(s.handleAnomalyWeather))

	// MAC vendor lookup
	mux.HandleFunc("GET /api/v2/macvendor/stats", s.auth.RequireAuth(s.handleMACVendorStats))
	mux.HandleFunc("GET /api/v2/macvendor/{mac}", s.auth.RequireAuth(s.handleMACVendorLookup))

	// RADIUS
	mux.HandleFunc("GET /api/v2/radius", s.auth.RequireAuth(s.handleRADIUSList))
	mux.HandleFunc("POST /api/v2/radius/test", s.auth.RequireAdmin(s.handleRADIUSTest))
	mux.HandleFunc("GET /api/v2/radius/{subnet}", s.auth.RequireAuth(s.handleRADIUSGetSubnet))
	mux.HandleFunc("PUT /api/v2/radius/{subnet}", s.auth.RequireAdmin(s.handleRADIUSSetSubnet))
	mux.HandleFunc("DELETE /api/v2/radius/{subnet}", s.auth.RequireAdmin(s.handleRADIUSDeleteSubnet))

	// Port automation
	mux.HandleFunc("GET /api/v2/portauto/rules", s.auth.RequireAuth(s.handlePortAutoGetRules))
	mux.HandleFunc("PUT /api/v2/portauto/rules", s.auth.RequireAdmin(s.handlePortAutoSetRules))
	mux.HandleFunc("POST /api/v2/portauto/test", s.auth.RequireAdmin(s.handlePortAutoTest))

	// HA
	mux.HandleFunc("GET /api/v2/ha/status", s.auth.RequireAuth(s.handleHAStatus))
	mux.HandleFunc("POST /api/v2/ha/failover", s.auth.RequireAdmin(s.handleHAFailover))

	// DNS proxy
	mux.HandleFunc("GET /api/v2/dns/stats", s.auth.RequireAuth(s.handleDNSStats))
	mux.HandleFunc("POST /api/v2/dns/cache/flush", s.auth.RequireAdmin(s.handleDNSFlushCache))
	mux.HandleFunc("GET /api/v2/dns/records", s.auth.RequireAuth(s.handleDNSListRecords))
	mux.HandleFunc("GET /api/v2/dns/lists", s.auth.RequireAuth(s.handleDNSListStatus))
	mux.HandleFunc("POST /api/v2/dns/lists/refresh", s.auth.RequireAdmin(s.handleDNSListRefresh))
	mux.HandleFunc("POST /api/v2/dns/lists/test", s.auth.RequireAuth(s.handleDNSListTest))
	mux.HandleFunc("GET /api/v2/dns/querylog", s.auth.RequireAuth(s.handleDNSQueryLog))
	mux.HandleFunc("GET /api/v2/dns/querylog/stream", s.auth.RequireAuth(s.handleDNSQueryLogStream))

	// Stats
	mux.HandleFunc("GET /api/v2/stats", s.auth.RequireAuth(s.handleGetStats))

	// Setup wizard (GET status is always open; POST endpoints locked after setup)
	mux.HandleFunc("GET /api/v2/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/v2/setup/ha", s.setupGuard(s.handleSetupHA))
	mux.HandleFunc("POST /api/v2/setup/config", s.setupGuard(s.handleSetupConfig))
	mux.HandleFunc("POST /api/v2/setup/complete", s.setupGuard(s.handleSetupComplete))

	// Backup & Restore
	mux.HandleFunc("GET /api/v2/backup", s.auth.RequireAdmin(s.handleBackupExport))
	mux.HandleFunc("POST /api/v2/backup/restore", s.auth.RequireAdmin(s.handleBackupRestore))

	// SPA fallback — serve index.html for all non-API paths
	mux.HandleFunc("/", s.handleSPA)
}

// UpdateConfig updates the runtime config pointer (called on live config reload).
func (s *Server) UpdateConfig(cfg *config.Config) {
	s.cfg = cfg
	// Refresh auth middleware with updated user list (includes DB users merged by BuildConfig)
	s.auth.UpdateUsers(cfg.API.Auth.Users)
}

// UpdatePools replaces the pool list (called on live config reload).
func (s *Server) UpdatePools(pools []*pool.Pool) {
	s.pools = pools
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

// primaryWebURL returns the primary node's web UI URL by combining the
// peer's IP with our own API listen port. Returns empty string if HA is
// not configured or the address can't be parsed.
func (s *Server) primaryWebURL() string {
	if !s.cfg.HA.Enabled || s.cfg.HA.PeerAddress == "" {
		return ""
	}
	peerHost, _, err := net.SplitHostPort(s.cfg.HA.PeerAddress)
	if err != nil {
		return ""
	}
	_, apiPort, err := net.SplitHostPort(s.cfg.API.Listen)
	if err != nil {
		apiPort = "8067"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(peerHost, apiPort))
}

// setupGuard wraps a handler to block setup wizard endpoints after setup is complete.
func (s *Server) setupGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfgStore != nil && s.cfgStore.IsSetupComplete() {
			JSONError(w, http.StatusForbidden, "setup_complete", "setup wizard is locked — setup already complete")
			return
		}
		next(w, r)
	}
}

// standbyGuard wraps a handler to block writes when this node is HA standby.
// Returns 409 Conflict with the primary's URL so the client can redirect.
func (s *Server) standbyGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.fsm != nil && !s.fsm.IsActive() {
			primaryURL := s.primaryWebURL()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":       "this node is standby — config changes must be made on the primary",
				"code":        "ha_standby",
				"primary_url": primaryURL,
			})
			return
		}
		next(w, r)
	}
}
