package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func newTestServer(t *testing.T) *Server {
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

	cfg := &config.Config{
		API: config.APIConfig{
			Listen: "127.0.0.1:0",
			Auth:   config.APIAuthConfig{},
		},
		Subnets: []config.SubnetConfig{
			{
				Network:    "192.168.1.0/24",
				Routers:    []string{"192.168.1.1"},
				DNSServers: []string{"8.8.8.8"},
			},
		},
	}

	return NewServer(cfg, store, nil, nil, nil, bus, logger)
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

func TestHandleListLeases(t *testing.T) {
	srv := newTestServer(t)

	// Add a lease
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	now := time.Now()
	srv.leaseStore.Put(&lease.Lease{
		IP:          net.IPv4(192, 168, 1, 100),
		MAC:         mac,
		Hostname:    "testhost",
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	})

	req := httptest.NewRequest("GET", "/api/v2/leases", nil)
	w := httptest.NewRecorder()
	srv.handleListLeases(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	leaseList, ok := resp["leases"].([]interface{})
	if !ok {
		t.Fatalf("expected leases array in response")
	}
	if len(leaseList) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leaseList))
	}
	first := leaseList[0].(map[string]interface{})
	if first["hostname"] != "testhost" {
		t.Errorf("hostname = %v, want testhost", first["hostname"])
	}
	if resp["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", resp["total"])
	}
}

func TestHandleGetLease(t *testing.T) {
	srv := newTestServer(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	now := time.Now()
	srv.leaseStore.Put(&lease.Lease{
		IP:          net.IPv4(192, 168, 1, 100),
		MAC:         mac,
		Hostname:    "testhost",
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	})

	req := httptest.NewRequest("GET", "/api/v2/leases/192.168.1.100", nil)
	req.SetPathValue("ip", "192.168.1.100")
	w := httptest.NewRecorder()
	srv.handleGetLease(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleGetLeaseNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/leases/10.0.0.1", nil)
	req.SetPathValue("ip", "10.0.0.1")
	w := httptest.NewRecorder()
	srv.handleGetLease(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetLeaseInvalidIP(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/leases/badip", nil)
	req.SetPathValue("ip", "badip")
	w := httptest.NewRecorder()
	srv.handleGetLease(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteLease(t *testing.T) {
	srv := newTestServer(t)

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	now := time.Now()
	srv.leaseStore.Put(&lease.Lease{
		IP:          net.IPv4(192, 168, 1, 100),
		MAC:         mac,
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       now,
		Expiry:      now.Add(time.Hour),
		LastUpdated: now,
	})

	req := httptest.NewRequest("DELETE", "/api/v2/leases/192.168.1.100", nil)
	req.SetPathValue("ip", "192.168.1.100")
	w := httptest.NewRecorder()
	srv.handleDeleteLease(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if srv.leaseStore.Count() != 0 {
		t.Error("lease should be deleted")
	}
}

func TestHandleListSubnets(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/subnets", nil)
	w := httptest.NewRecorder()
	srv.handleListSubnets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var subnets []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &subnets)
	if len(subnets) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(subnets))
	}
}

func TestHandleListConflictsNil(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/conflicts", nil)
	w := httptest.NewRecorder()
	srv.handleListConflicts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleGetStats(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/stats", nil)
	w := httptest.NewRecorder()
	srv.handleGetStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats["timestamp"] == nil {
		t.Error("expected timestamp in stats")
	}
}

func TestHandleSPA(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleSPA(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestHandleSPANotForAPI(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v2/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleSPA(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("SPA should return 404 for /api/ paths, got %d", w.Code)
	}
}

func TestRedactSecrets(t *testing.T) {
	m := map[string]interface{}{
		"server": map[string]interface{}{
			"bind": "0.0.0.0:67",
		},
		"ddns": map[string]interface{}{
			"tsig_secret": "super-secret",
			"api_key":     "my-api-key",
			"zone":        "example.com.",
		},
		"api": map[string]interface{}{
			"auth_token": "bearer-token",
			"users": []interface{}{
				map[string]interface{}{
					"username":      "admin",
					"password_hash": "$2y$10$hash",
				},
			},
		},
	}

	redactSecrets(m)

	ddns := m["ddns"].(map[string]interface{})
	if ddns["tsig_secret"] != "***REDACTED***" {
		t.Errorf("tsig_secret not redacted: %v", ddns["tsig_secret"])
	}
	if ddns["api_key"] != "***REDACTED***" {
		t.Errorf("api_key not redacted: %v", ddns["api_key"])
	}
	if ddns["zone"] != "example.com." {
		t.Errorf("zone should not be redacted: %v", ddns["zone"])
	}

	apiCfg := m["api"].(map[string]interface{})
	if apiCfg["auth_token"] != "***REDACTED***" {
		t.Errorf("auth_token not redacted: %v", apiCfg["auth_token"])
	}
}
