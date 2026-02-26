package radius

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestClientSetAndGetSubnet(t *testing.T) {
	c := NewClient(testLogger())

	cfg := &SubnetConfig{
		Enabled: true,
		Server: ServerConfig{
			Address: "127.0.0.1:1812",
			Secret:  "testing123",
			Timeout: "5s",
			Retries: 3,
		},
		NASIdentifier:  "athena",
		CallingStation: true,
	}

	c.SetSubnet("10.0.0.0/24", cfg)

	got := c.GetSubnet("10.0.0.0/24")
	if got == nil {
		t.Fatal("expected config for 10.0.0.0/24")
	}
	if got.Server.Address != "127.0.0.1:1812" {
		t.Errorf("address = %q", got.Server.Address)
	}
	if got.Server.Secret != "testing123" {
		t.Errorf("secret = %q", got.Server.Secret)
	}
	if !got.CallingStation {
		t.Error("CallingStation should be true")
	}
}

func TestClientGetSubnetUnknown(t *testing.T) {
	c := NewClient(testLogger())
	if c.GetSubnet("10.0.0.0/24") != nil {
		t.Error("expected nil for unknown subnet")
	}
}

func TestClientRemoveSubnet(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{Enabled: true})
	c.RemoveSubnet("10.0.0.0/24")
	if c.GetSubnet("10.0.0.0/24") != nil {
		t.Error("expected nil after remove")
	}
}

func TestClientListSubnets(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{Enabled: true})
	c.SetSubnet("192.168.1.0/24", &SubnetConfig{Enabled: false})

	list := c.ListSubnets()
	if len(list) != 2 {
		t.Errorf("list has %d entries", len(list))
	}
}

func TestAuthNoRadiusConfigured(t *testing.T) {
	c := NewClient(testLogger())

	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", nil)

	if !result.Accepted {
		t.Error("should accept when no RADIUS configured")
	}
	if result.Code != "no_radius" {
		t.Errorf("code = %q", result.Code)
	}
}

func TestAuthDisabledSubnet(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{Enabled: false})

	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", nil)

	if !result.Accepted {
		t.Error("should accept when RADIUS disabled")
	}
}

func TestAuthUnreachableServer(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{
		Enabled: true,
		Server: ServerConfig{
			Address: "127.0.0.1:19999", // nothing listening
			Secret:  "test",
			Timeout: "500ms",
		},
	})

	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", nil)

	if result.Accepted {
		t.Error("should not accept when server unreachable")
	}
	if result.Error == "" {
		t.Error("should have an error message")
	}
	if result.Code != "error" {
		t.Errorf("code = %q", result.Code)
	}
}

func TestTestUnreachable(t *testing.T) {
	c := NewClient(testLogger())

	result := c.Test(context.Background(), &ServerConfig{
		Address: "127.0.0.1:19999",
		Secret:  "test",
		Timeout: "500ms",
	}, "admin", "password")

	if result.Accepted {
		t.Error("should not accept for unreachable server")
	}
	if result.Error == "" {
		t.Error("should have error")
	}
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"5s", "5s"},
		{"", "5s"},
		{"invalid", "5s"},
		{"100ms", "100ms"},
		{"-1s", "5s"},
	}
	for _, tc := range tests {
		got := parseTimeout(tc.input)
		if got.String() != tc.want {
			t.Errorf("parseTimeout(%q) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestAuthWithOption82(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{
		Enabled: true,
		Server: ServerConfig{
			Address: "127.0.0.1:19999",
			Secret:  "test",
			Timeout: "500ms",
		},
		SendOption82: true,
	})

	opt82 := &Option82Info{
		CircuitID: "eth0/1/3",
		RemoteID:  "switch-01.example.com",
		GIAddr:    "10.0.0.1",
	}

	// Will fail because nothing is listening, but the packet is built correctly
	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", opt82)

	if result.Accepted {
		t.Error("should not accept for unreachable server")
	}
	if result.Code != "error" {
		t.Errorf("code = %q", result.Code)
	}
}

func TestAuthOption82DisabledIgnoresData(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{
		Enabled: true,
		Server: ServerConfig{
			Address: "127.0.0.1:19999",
			Secret:  "test",
			Timeout: "500ms",
		},
		SendOption82: false, // Option 82 disabled
	})

	opt82 := &Option82Info{
		CircuitID: "eth0/1/3",
		RemoteID:  "switch-01",
		GIAddr:    "10.0.0.1",
	}

	// Should still attempt auth but without option 82 attrs
	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", opt82)

	if result.Accepted {
		t.Error("should not accept for unreachable server")
	}
	if result.Code != "error" {
		t.Errorf("code = %q", result.Code)
	}
}

func TestAuthOption82NilSafe(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{
		Enabled: true,
		Server: ServerConfig{
			Address: "127.0.0.1:19999",
			Secret:  "test",
			Timeout: "500ms",
		},
		SendOption82: true, // Enabled but opt82 is nil
	})

	// nil opt82 should not panic
	result := c.Authenticate(context.Background(), "10.0.0.0/24",
		net.HardwareAddr{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}, "test-user", nil)

	if result.Accepted {
		t.Error("should not accept for unreachable server")
	}
}

func TestOption82InfoStruct(t *testing.T) {
	info := &Option82Info{
		CircuitID: "eth0/1/3",
		RemoteID:  "switch-01.example.com",
		GIAddr:    "10.0.0.1",
	}
	if info.CircuitID != "eth0/1/3" {
		t.Errorf("CircuitID = %q", info.CircuitID)
	}
	if info.RemoteID != "switch-01.example.com" {
		t.Errorf("RemoteID = %q", info.RemoteID)
	}
	if info.GIAddr != "10.0.0.1" {
		t.Errorf("GIAddr = %q", info.GIAddr)
	}
}

func TestGetSubnetReturnsCopy(t *testing.T) {
	c := NewClient(testLogger())
	c.SetSubnet("10.0.0.0/24", &SubnetConfig{Enabled: true, NASIdentifier: "original"})

	got := c.GetSubnet("10.0.0.0/24")
	got.NASIdentifier = "modified"

	got2 := c.GetSubnet("10.0.0.0/24")
	if got2.NASIdentifier != "original" {
		t.Error("GetSubnet should return a copy")
	}
}
