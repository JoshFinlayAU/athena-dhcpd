package ddns

import (
	"net"
	"testing"
)

func TestEnsureDot(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"", ""},
		{"host.example.com", "host.example.com."},
	}
	for _, tt := range tests {
		got := ensureDot(tt.input)
		if got != tt.want {
			t.Errorf("ensureDot(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReverseIPName(t *testing.T) {
	tests := []struct {
		ip   net.IP
		want string
	}{
		{net.IPv4(192, 168, 1, 100), "100.1.168.192.in-addr.arpa"},
		{net.IPv4(10, 0, 0, 1), "1.0.0.10.in-addr.arpa"},
		{net.IPv4(172, 16, 254, 3), "3.254.16.172.in-addr.arpa"},
		{net.IPv4(0, 0, 0, 0), "0.0.0.0.in-addr.arpa"},
	}
	for _, tt := range tests {
		got := ReverseIPName(tt.ip)
		if got != tt.want {
			t.Errorf("ReverseIPName(%s) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestBuildFQDN(t *testing.T) {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")

	tests := []struct {
		name       string
		clientFQDN string
		hostname   string
		domain     string
		mac        net.HardwareAddr
		fallback   bool
		want       string
	}{
		{"client FQDN wins", "client.example.com", "host", "example.com", mac, true, "client.example.com."},
		{"hostname + domain", "", "myhost", "example.com", mac, true, "myhost.example.com."},
		{"hostname only", "", "myhost", "", mac, true, "myhost"},
		{"MAC fallback with domain", "", "", "example.com", mac, true, "00-11-22-33-44-55.example.com."},
		{"MAC fallback no domain", "", "", "", mac, true, "00-11-22-33-44-55"},
		{"no fallback returns empty", "", "", "example.com", mac, false, ""},
		{"all empty", "", "", "", nil, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFQDN(tt.clientFQDN, tt.hostname, tt.domain, tt.mac, tt.fallback)
			if got != tt.want {
				t.Errorf("BuildFQDN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"valid-host", "valid-host"},
		{"UPPER.Case", "upper.case"},
		{"host name with spaces", "hostnamewithspaces"},
		{"host_with_underscore", "hostwithunderscore"},
		{"-leading-hyphen", "leading-hyphen"},
		{"trailing-dot.", "trailing-dot"},
		{"special!@#chars", "specialchars"},
		{"", ""},
		{"a.b.c", "a.b.c"},
	}

	for _, tt := range tests {
		got := SanitizeHostname(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeHostname(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
