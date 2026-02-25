package config

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const minimalConfig = `
[server]
interface = "eth0"
bind_address = "0.0.0.0:67"
server_id = "192.168.1.1"
log_level = "info"
lease_db = "/tmp/test.db"

[defaults]
lease_time = "8h"
renewal_time = "4h"
rebind_time = "7h"

[[subnet]]
network = "192.168.1.0/24"
routers = ["192.168.1.1"]
dns_servers = ["8.8.8.8"]

  [[subnet.pool]]
  range_start = "192.168.1.100"
  range_end = "192.168.1.200"
`

func TestLoadMinimalConfig(t *testing.T) {
	path := writeTestConfig(t, minimalConfig)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Server.Interface != "eth0" {
		t.Errorf("Interface = %q, want %q", cfg.Server.Interface, "eth0")
	}
	if cfg.Server.BindAddress != "0.0.0.0:67" {
		t.Errorf("BindAddress = %q, want %q", cfg.Server.BindAddress, "0.0.0.0:67")
	}
	if cfg.Server.ServerID != "192.168.1.1" {
		t.Errorf("ServerID = %q, want %q", cfg.Server.ServerID, "192.168.1.1")
	}
	if len(cfg.Subnets) != 1 {
		t.Fatalf("Subnets = %d, want 1", len(cfg.Subnets))
	}
	if cfg.Subnets[0].Network != "192.168.1.0/24" {
		t.Errorf("Subnet network = %q, want %q", cfg.Subnets[0].Network, "192.168.1.0/24")
	}
	if len(cfg.Subnets[0].Pools) != 1 {
		t.Fatalf("Pools = %d, want 1", len(cfg.Subnets[0].Pools))
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path.toml")
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestLoadConfigInvalidTOML(t *testing.T) {
	path := writeTestConfig(t, "this is not valid toml {{{{")
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestValidateInvalidServerID(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindAddress: "0.0.0.0:67",
			ServerID:    "not-an-ip",
			LeaseDB:     "/tmp/test.db",
		},
		Defaults: DefaultsConfig{
			LeaseTime:   "8h",
			RenewalTime: "4h",
			RebindTime:  "7h",
		},
	}
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid server_id")
	}
}

func TestValidateInvalidSubnetNetwork(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindAddress: "0.0.0.0:67",
			ServerID:    "192.168.1.1",
			LeaseDB:     "/tmp/test.db",
		},
		Defaults: DefaultsConfig{
			LeaseTime:   "8h",
			RenewalTime: "4h",
			RebindTime:  "7h",
		},
		Subnets: []SubnetConfig{
			{Network: "not-a-cidr"},
		},
	}
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid subnet network")
	}
}

func TestValidatePoolRangeOutsideNetwork(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindAddress: "0.0.0.0:67",
			ServerID:    "192.168.1.1",
			LeaseDB:     "/tmp/test.db",
		},
		Defaults: DefaultsConfig{
			LeaseTime:   "8h",
			RenewalTime: "4h",
			RebindTime:  "7h",
		},
		Subnets: []SubnetConfig{
			{
				Network: "192.168.1.0/24",
				Pools: []PoolConfig{
					{RangeStart: "10.0.0.1", RangeEnd: "10.0.0.100"},
				},
			},
		},
	}
	if err := validate(cfg); err == nil {
		t.Error("expected error for pool range outside network")
	}
}

func TestValidateHAConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindAddress: "0.0.0.0:67",
			ServerID:    "192.168.1.1",
			LeaseDB:     "/tmp/test.db",
		},
		Defaults: DefaultsConfig{
			LeaseTime:   "8h",
			RenewalTime: "4h",
			RebindTime:  "7h",
		},
		Subnets: []SubnetConfig{
			{Network: "192.168.1.0/24"},
		},
		HA: HAConfig{
			Enabled: true,
			Role:    "invalid_role",
		},
	}
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid HA role")
	}
}

func TestValidateDDNSConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindAddress: "0.0.0.0:67",
			ServerID:    "192.168.1.1",
			LeaseDB:     "/tmp/test.db",
		},
		Defaults: DefaultsConfig{
			LeaseTime:   "8h",
			RenewalTime: "4h",
			RebindTime:  "7h",
		},
		Subnets: []SubnetConfig{
			{Network: "192.168.1.0/24"},
		},
		DDNS: DDNSConfig{
			Enabled: true,
			Forward: DDNSZoneConfig{
				Zone:   "", // Missing zone
				Method: "rfc2136",
			},
		},
	}
	if err := validate(cfg); err == nil {
		t.Error("expected error for missing DDNS forward zone")
	}
}

func TestGetLeaseTime(t *testing.T) {
	cfg := &Config{
		Defaults: DefaultsConfig{LeaseTime: "8h"},
		Subnets: []SubnetConfig{
			{LeaseTime: "12h"},
			{LeaseTime: ""},
		},
	}

	// Subnet with explicit lease time
	d := cfg.GetLeaseTime(0)
	if d != 12*time.Hour {
		t.Errorf("GetLeaseTime(0) = %v, want 12h", d)
	}

	// Subnet without explicit lease time — falls back to default
	d2 := cfg.GetLeaseTime(1)
	if d2 != 8*time.Hour {
		t.Errorf("GetLeaseTime(1) = %v, want 8h", d2)
	}

	// Out of bounds — falls back to default
	d3 := cfg.GetLeaseTime(99)
	if d3 != 8*time.Hour {
		t.Errorf("GetLeaseTime(99) = %v, want 8h", d3)
	}
}

func TestGetRenewalTime(t *testing.T) {
	cfg := &Config{
		Defaults: DefaultsConfig{RenewalTime: "4h"},
		Subnets: []SubnetConfig{
			{RenewalTime: "6h"},
		},
	}

	d := cfg.GetRenewalTime(0)
	if d != 6*time.Hour {
		t.Errorf("GetRenewalTime(0) = %v, want 6h", d)
	}

	d2 := cfg.GetRenewalTime(99)
	if d2 != 4*time.Hour {
		t.Errorf("GetRenewalTime(99) = %v, want 4h", d2)
	}
}

func TestGetRebindTime(t *testing.T) {
	cfg := &Config{
		Defaults: DefaultsConfig{RebindTime: "7h"},
		Subnets: []SubnetConfig{
			{RebindTime: "10h30m"},
		},
	}

	d := cfg.GetRebindTime(0)
	if d != 10*time.Hour+30*time.Minute {
		t.Errorf("GetRebindTime(0) = %v, want 10h30m", d)
	}
}

func TestServerIP(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{ServerID: "192.168.1.1"},
	}
	ip := cfg.ServerIP()
	if ip == nil || !ip.Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("ServerIP() = %v, want 192.168.1.1", ip)
	}

	cfg2 := &Config{
		Server: ServerConfig{ServerID: ""},
	}
	if cfg2.ServerIP() != nil {
		t.Error("ServerIP() should return nil for empty server_id")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.LogLevel != "info" {
		t.Errorf("default LogLevel = %q, want %q", cfg.Server.LogLevel, "info")
	}
	if cfg.Defaults.LeaseTime == "" {
		t.Error("default LeaseTime should be set")
	}
	if cfg.ConflictDetection.ProbeStrategy != "sequential" {
		t.Errorf("default ProbeStrategy = %q, want %q", cfg.ConflictDetection.ProbeStrategy, "sequential")
	}
}
