package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/ha"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"

	bolt "go.etcd.io/bbolt"
)

// newTestServerWithConflicts creates a test server with a conflict table.
func newTestServerWithConflicts(t *testing.T) (*Server, *conflict.Table) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := events.NewBus(100, logger)
	go bus.Start()
	t.Cleanup(func() { bus.Stop() })

	dir := t.TempDir()
	storePath := filepath.Join(dir, "test.db")
	store, err := lease.NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ct, err := conflict.NewTable(store.DB(), time.Hour, 3)
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	cfg := &config.Config{
		API: config.APIConfig{
			Enabled: true,
			Listen:  "127.0.0.1:0",
			WebUI:   true,
			Auth:    config.APIAuthConfig{},
		},
		Subnets: []config.SubnetConfig{
			{
				Network:    "192.168.1.0/24",
				Routers:    []string{"192.168.1.1"},
				DNSServers: []string{"8.8.8.8"},
				Reservations: []config.ReservationConfig{
					{MAC: "00:11:22:33:44:55", IP: "192.168.1.10", Hostname: "switch-core"},
				},
			},
		},
		Hooks: config.HooksConfig{
			Scripts: []config.ScriptHook{
				{Name: "test-script", Events: []string{"lease.ack"}, Command: "/bin/true"},
			},
			Webhooks: []config.WebhookHook{
				{Name: "test-webhook", Events: []string{"lease.ack"}, URL: "http://localhost:9999/hook"},
			},
		},
	}

	return NewServer(cfg, store, nil, ct, nil, bus, logger), ct
}

// --- Lease Filtering Tests ---

func TestHandleListLeasesFiltering(t *testing.T) {
	srv := newTestServer(t)

	mac1, _ := net.ParseMAC("00:11:22:33:44:55")
	mac2, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	now := time.Now()

	srv.leaseStore.Put(&lease.Lease{
		IP: net.IPv4(192, 168, 1, 100), MAC: mac1, Hostname: "host-a",
		Subnet: "192.168.1.0/24", State: dhcpv4.LeaseStateActive,
		Start: now, Expiry: now.Add(time.Hour), LastUpdated: now,
	})
	srv.leaseStore.Put(&lease.Lease{
		IP: net.IPv4(192, 168, 1, 101), MAC: mac2, Hostname: "host-b",
		Subnet: "10.0.0.0/24", State: dhcpv4.LeaseStateActive,
		Start: now, Expiry: now.Add(time.Hour), LastUpdated: now,
	})

	// Filter by subnet
	req := httptest.NewRequest("GET", "/api/v2/leases?subnet=192.168.1.0/24", nil)
	w := httptest.NewRecorder()
	srv.handleListLeases(w, req)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	leases := resp["leases"].([]interface{})
	if len(leases) != 1 {
		t.Errorf("subnet filter: got %d leases, want 1", len(leases))
	}

	// Filter by MAC
	req2 := httptest.NewRequest("GET", "/api/v2/leases?mac=aa:bb", nil)
	w2 := httptest.NewRecorder()
	srv.handleListLeases(w2, req2)
	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	leases2 := resp2["leases"].([]interface{})
	if len(leases2) != 1 {
		t.Errorf("mac filter: got %d leases, want 1", len(leases2))
	}

	// Filter by hostname
	req3 := httptest.NewRequest("GET", "/api/v2/leases?hostname=host-a", nil)
	w3 := httptest.NewRecorder()
	srv.handleListLeases(w3, req3)
	var resp3 map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &resp3)
	leases3 := resp3["leases"].([]interface{})
	if len(leases3) != 1 {
		t.Errorf("hostname filter: got %d leases, want 1", len(leases3))
	}
}

func TestHandleListLeasesPagination(t *testing.T) {
	srv := newTestServer(t)

	now := time.Now()
	macs := []string{"00:11:22:33:44:01", "00:11:22:33:44:02", "00:11:22:33:44:03", "00:11:22:33:44:04", "00:11:22:33:44:05"}
	for i := 0; i < 5; i++ {
		mac, _ := net.ParseMAC(macs[i])
		srv.leaseStore.Put(&lease.Lease{
			IP: net.IPv4(192, 168, 1, byte(100+i)), MAC: mac,
			Subnet: "192.168.1.0/24", State: dhcpv4.LeaseStateActive,
			Start: now, Expiry: now.Add(time.Hour), LastUpdated: now,
		})
	}

	req := httptest.NewRequest("GET", "/api/v2/leases?page=2&page_size=2", nil)
	w := httptest.NewRecorder()
	srv.handleListLeases(w, req)

	if w.Header().Get("X-Total-Count") != "5" {
		t.Errorf("X-Total-Count = %q, want 5", w.Header().Get("X-Total-Count"))
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	leases := resp["leases"].([]interface{})
	if len(leases) != 2 {
		t.Errorf("pagination: got %d leases, want 2", len(leases))
	}
	if resp["total"].(float64) != 5 {
		t.Errorf("total = %v, want 5", resp["total"])
	}
	if resp["page"].(float64) != 2 {
		t.Errorf("page = %v, want 2", resp["page"])
	}
}

func TestHandleExportLeases(t *testing.T) {
	srv := newTestServer(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	now := time.Now()
	srv.leaseStore.Put(&lease.Lease{
		IP: net.IPv4(192, 168, 1, 100), MAC: mac, Hostname: "testhost",
		Subnet: "192.168.1.0/24", State: dhcpv4.LeaseStateActive,
		Start: now, Expiry: now.Add(time.Hour), LastUpdated: now,
	})

	req := httptest.NewRequest("GET", "/api/v2/leases/export", nil)
	w := httptest.NewRecorder()
	srv.handleExportLeases(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "192.168.1.100") {
		t.Error("CSV should contain lease IP")
	}
	if !strings.Contains(body, "testhost") {
		t.Error("CSV should contain hostname")
	}
}

// --- Reservation Tests ---

func TestHandleListReservations(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("GET", "/api/v2/reservations", nil)
	w := httptest.NewRecorder()
	srv.handleListReservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var res []reservationResponse
	json.Unmarshal(w.Body.Bytes(), &res)
	if len(res) != 1 {
		t.Fatalf("got %d reservations, want 1", len(res))
	}
	if res[0].MAC != "00:11:22:33:44:55" {
		t.Errorf("MAC = %q, want 00:11:22:33:44:55", res[0].MAC)
	}
}

func TestHandleCreateReservation(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	body := `{"subnet_index": 0, "mac": "aa:bb:cc:dd:ee:ff", "ip": "192.168.1.20", "hostname": "new-host"}`
	req := httptest.NewRequest("POST", "/api/v2/reservations", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreateReservation(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body=%s", w.Code, http.StatusCreated, w.Body.String())
	}

	if len(srv.cfg.Subnets[0].Reservations) != 2 {
		t.Errorf("expected 2 reservations, got %d", len(srv.cfg.Subnets[0].Reservations))
	}
}

func TestHandleCreateReservationValidation(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	// Missing MAC and identifier
	body := `{"subnet_index": 0, "ip": "192.168.1.20"}`
	req := httptest.NewRequest("POST", "/api/v2/reservations", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreateReservation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteReservation(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("DELETE", "/api/v2/reservations/0", nil)
	req.SetPathValue("id", "0")
	w := httptest.NewRecorder()
	srv.handleDeleteReservation(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(srv.cfg.Subnets[0].Reservations) != 0 {
		t.Errorf("expected 0 reservations, got %d", len(srv.cfg.Subnets[0].Reservations))
	}
}

func TestHandleDeleteReservationNotFound(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("DELETE", "/api/v2/reservations/99", nil)
	req.SetPathValue("id", "99")
	w := httptest.NewRecorder()
	srv.handleDeleteReservation(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleExportReservations(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("GET", "/api/v2/reservations/export", nil)
	w := httptest.NewRecorder()
	srv.handleExportReservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "switch-core") {
		t.Error("CSV should contain reservation hostname")
	}
}

// --- Conflict Tests ---

func TestHandleGetConflict(t *testing.T) {
	srv, ct := newTestServerWithConflicts(t)

	ct.Add(net.IPv4(192, 168, 1, 50), "arp_probe", "aa:bb:cc:11:22:33", "192.168.1.0/24")

	req := httptest.NewRequest("GET", "/api/v2/conflicts/192.168.1.50", nil)
	req.SetPathValue("ip", "192.168.1.50")
	w := httptest.NewRecorder()
	srv.handleGetConflict(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var rec map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &rec)
	if rec["detection_method"] != "arp_probe" {
		t.Errorf("detection_method = %v, want arp_probe", rec["detection_method"])
	}
}

func TestHandleGetConflictNotFound(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("GET", "/api/v2/conflicts/10.0.0.1", nil)
	req.SetPathValue("ip", "10.0.0.1")
	w := httptest.NewRecorder()
	srv.handleGetConflict(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleExcludeConflict(t *testing.T) {
	srv, ct := newTestServerWithConflicts(t)

	req := httptest.NewRequest("POST", "/api/v2/conflicts/192.168.1.60/exclude", nil)
	req.SetPathValue("ip", "192.168.1.60")
	w := httptest.NewRecorder()
	srv.handleExcludeConflict(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if !ct.IsConflicted(net.IPv4(192, 168, 1, 60)) {
		t.Error("IP should be conflicted after exclude")
	}
}

func TestHandleConflictHistory(t *testing.T) {
	srv, ct := newTestServerWithConflicts(t)

	ct.Add(net.IPv4(192, 168, 1, 50), "arp_probe", "", "192.168.1.0/24")
	ct.Resolve(net.IPv4(192, 168, 1, 50))

	req := httptest.NewRequest("GET", "/api/v2/conflicts/history", nil)
	w := httptest.NewRecorder()
	srv.handleConflictHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var history []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &history)
	if len(history) != 1 {
		t.Errorf("expected 1 resolved conflict, got %d", len(history))
	}
}

func TestHandleConflictStats(t *testing.T) {
	srv, ct := newTestServerWithConflicts(t)

	ct.Add(net.IPv4(192, 168, 1, 50), "arp_probe", "", "192.168.1.0/24")
	ct.Add(net.IPv4(192, 168, 1, 51), "icmp_probe", "", "192.168.1.0/24")

	req := httptest.NewRequest("GET", "/api/v2/conflicts/stats", nil)
	w := httptest.NewRecorder()
	srv.handleConflictStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats["enabled"] != true {
		t.Error("expected enabled=true")
	}
	if stats["total_active"].(float64) != 2 {
		t.Errorf("total_active = %v, want 2", stats["total_active"])
	}
}

// --- Config Tests ---

func TestHandleGetConfigRaw(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	os.WriteFile(cfgPath, []byte("[server]\nbind_address = \"0.0.0.0:67\"\n"), 0644)

	srv := newTestServer(t)
	srv.configPath = cfgPath

	req := httptest.NewRequest("GET", "/api/v2/config/raw", nil)
	w := httptest.NewRecorder()
	srv.handleGetConfigRaw(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "bind_address") {
		t.Error("should contain config content")
	}
}

func TestHandleGetConfigRawNoPath(t *testing.T) {
	srv := newTestServer(t)
	// configPath is empty

	req := httptest.NewRequest("GET", "/api/v2/config/raw", nil)
	w := httptest.NewRecorder()
	srv.handleGetConfigRaw(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleValidateConfig(t *testing.T) {
	srv := newTestServer(t)

	// Valid TOML
	body := `{"config": "[server]\nbind_address = \"0.0.0.0:67\"\n"}`
	req := httptest.NewRequest("POST", "/api/v2/config/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleValidateConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
}

func TestHandleValidateConfigInvalid(t *testing.T) {
	srv := newTestServer(t)

	body := `{"config": "this is not valid toml {{{{"}`
	req := httptest.NewRequest("POST", "/api/v2/config/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleValidateConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] != false {
		t.Errorf("expected valid=false, got %v", resp["valid"])
	}
}

// --- Events & Hooks Tests ---

func TestHandleListEvents(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/events", nil)
	w := httptest.NewRecorder()
	srv.handleListEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleListHooks(t *testing.T) {
	srv, _ := newTestServerWithConflicts(t)

	req := httptest.NewRequest("GET", "/api/v2/hooks", nil)
	w := httptest.NewRecorder()
	srv.handleListHooks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var hooks []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &hooks)
	if len(hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(hooks))
	}
}

func TestHandleTestHook(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/v2/hooks/test", nil)
	w := httptest.NewRecorder()
	srv.handleTestHook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// --- HA Tests ---

func TestHandleHAStatusDisabled(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/ha/status", nil)
	w := httptest.NewRecorder()
	srv.handleHAStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", resp["enabled"])
	}
}

func TestHandleHAStatusEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := events.NewBus(100, logger)
	go bus.Start()

	fsm := ha.NewFSM("primary", 10*time.Second, bus, logger)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "test.db")
	store, _ := lease.NewStore(storePath)

	cfg := &config.Config{
		API: config.APIConfig{Enabled: true, Listen: "127.0.0.1:0", Auth: config.APIAuthConfig{}},
		HA:  config.HAConfig{Enabled: true, PeerAddress: "10.0.0.2:6740", ListenAddress: "0.0.0.0:6740"},
	}

	srv := NewServer(cfg, store, nil, nil, nil, bus, logger, WithFSM(fsm))

	req := httptest.NewRequest("GET", "/api/v2/ha/status", nil)
	w := httptest.NewRecorder()
	srv.handleHAStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", resp["enabled"])
	}
	if resp["role"] != "primary" {
		t.Errorf("role = %v, want primary", resp["role"])
	}

	bus.Stop()
	store.Close()
}

func TestHandleHAFailoverDisabled(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/v2/ha/failover", nil)
	w := httptest.NewRecorder()
	srv.handleHAFailover(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleHAFailoverEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := events.NewBus(100, logger)
	go bus.Start()

	fsm := ha.NewFSM("secondary", 10*time.Second, bus, logger)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "test.db")
	store, _ := lease.NewStore(storePath)

	cfg := &config.Config{
		API: config.APIConfig{Enabled: true, Listen: "127.0.0.1:0", Auth: config.APIAuthConfig{}},
	}

	srv := NewServer(cfg, store, nil, nil, nil, bus, logger, WithFSM(fsm))

	req := httptest.NewRequest("POST", "/api/v2/ha/failover", nil)
	w := httptest.NewRecorder()
	srv.handleHAFailover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if !fsm.IsActive() {
		t.Error("FSM should be active after failover")
	}

	bus.Stop()
	store.Close()
}

// --- Metrics Tests ---

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer(t)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	handler := newMetricsMiddleware(mux)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "athena_dhcpd_") {
		t.Error("/metrics should contain athena_dhcpd_ metrics")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("/metrics should contain go runtime metrics")
	}
}

func TestMetricsMiddlewareTracksRequests(t *testing.T) {
	srv := newTestServer(t)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	handler := newMetricsMiddleware(mux)

	// Make a health request through the middleware
	req := httptest.NewRequest("GET", "/api/v2/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}

	// Now check that the metrics endpoint shows the request was counted
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	body := w2.Body.String()
	if !strings.Contains(body, "athena_dhcpd_api_requests_total") {
		t.Error("/metrics should contain api_requests_total after a request")
	}
}

// Ensure unused imports don't cause issues
var _ bolt.DB
