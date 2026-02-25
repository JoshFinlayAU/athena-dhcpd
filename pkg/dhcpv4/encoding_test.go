package dhcpv4

import (
	"net"
	"testing"
)

func TestIPToUint32(t *testing.T) {
	tests := []struct {
		ip   net.IP
		want uint32
	}{
		{net.IPv4(0, 0, 0, 0), 0},
		{net.IPv4(255, 255, 255, 255), 0xFFFFFFFF},
		{net.IPv4(192, 168, 1, 1), 0xC0A80101},
		{net.IPv4(10, 0, 0, 1), 0x0A000001},
		{net.IPv4(172, 16, 0, 1), 0xAC100001},
	}
	for _, tt := range tests {
		got := IPToUint32(tt.ip)
		if got != tt.want {
			t.Errorf("IPToUint32(%s) = 0x%08X, want 0x%08X", tt.ip, got, tt.want)
		}
	}
}

func TestUint32ToIP(t *testing.T) {
	tests := []struct {
		u    uint32
		want net.IP
	}{
		{0, net.IPv4(0, 0, 0, 0)},
		{0xFFFFFFFF, net.IPv4(255, 255, 255, 255)},
		{0xC0A80101, net.IPv4(192, 168, 1, 1)},
	}
	for _, tt := range tests {
		got := Uint32ToIP(tt.u)
		if !got.Equal(tt.want) {
			t.Errorf("Uint32ToIP(0x%08X) = %s, want %s", tt.u, got, tt.want)
		}
	}
}

func TestIPRoundTrip(t *testing.T) {
	ips := []net.IP{
		net.IPv4(192, 168, 1, 100),
		net.IPv4(10, 0, 0, 1),
		net.IPv4(172, 16, 254, 254),
		net.IPv4(0, 0, 0, 0),
		net.IPv4(255, 255, 255, 255),
	}
	for _, ip := range ips {
		u := IPToUint32(ip)
		got := Uint32ToIP(u)
		if !got.Equal(ip) {
			t.Errorf("roundtrip failed: %s → 0x%08X → %s", ip, u, got)
		}
	}
}

func TestIPToBytes(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 1)
	b := IPToBytes(ip)
	if len(b) != 4 {
		t.Fatalf("IPToBytes length = %d, want 4", len(b))
	}
	if b[0] != 192 || b[1] != 168 || b[2] != 1 || b[3] != 1 {
		t.Errorf("IPToBytes(%s) = %v, want [192 168 1 1]", ip, b)
	}
}

func TestBytesToIP(t *testing.T) {
	b := []byte{10, 0, 0, 1}
	ip := BytesToIP(b)
	expected := net.IPv4(10, 0, 0, 1)
	if !ip.Equal(expected) {
		t.Errorf("BytesToIP(%v) = %s, want %s", b, ip, expected)
	}

	// Short slice
	if got := BytesToIP([]byte{1, 2}); got != nil {
		t.Errorf("BytesToIP(short) = %s, want nil", got)
	}
}

func TestIPListToBytes(t *testing.T) {
	ips := []net.IP{net.IPv4(8, 8, 8, 8), net.IPv4(8, 8, 4, 4)}
	b := IPListToBytes(ips)
	if len(b) != 8 {
		t.Fatalf("IPListToBytes length = %d, want 8", len(b))
	}
	if b[0] != 8 || b[1] != 8 || b[2] != 8 || b[3] != 8 {
		t.Errorf("first IP bytes wrong: %v", b[:4])
	}
	if b[4] != 8 || b[5] != 8 || b[6] != 4 || b[7] != 4 {
		t.Errorf("second IP bytes wrong: %v", b[4:])
	}
}

func TestBytesToIPList(t *testing.T) {
	b := []byte{192, 168, 1, 1, 10, 0, 0, 1}
	ips, err := BytesToIPList(b)
	if err != nil {
		t.Fatalf("BytesToIPList error: %v", err)
	}
	if len(ips) != 2 {
		t.Fatalf("BytesToIPList length = %d, want 2", len(ips))
	}
	if !ips[0].Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("first IP = %s, want 192.168.1.1", ips[0])
	}
	if !ips[1].Equal(net.IPv4(10, 0, 0, 1)) {
		t.Errorf("second IP = %s, want 10.0.0.1", ips[1])
	}

	// Not multiple of 4
	_, err = BytesToIPList([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for non-multiple-of-4 bytes, got nil")
	}
}

func TestUint32ToBytes(t *testing.T) {
	b := Uint32ToBytes(0x12345678)
	if len(b) != 4 {
		t.Fatalf("Uint32ToBytes length = %d, want 4", len(b))
	}
	if b[0] != 0x12 || b[1] != 0x34 || b[2] != 0x56 || b[3] != 0x78 {
		t.Errorf("Uint32ToBytes(0x12345678) = %v", b)
	}
}

func TestBytesToUint32(t *testing.T) {
	got, err := BytesToUint32([]byte{0x12, 0x34, 0x56, 0x78})
	if err != nil {
		t.Fatalf("BytesToUint32 error: %v", err)
	}
	if got != 0x12345678 {
		t.Errorf("BytesToUint32 = 0x%08X, want 0x12345678", got)
	}
	_, err = BytesToUint32([]byte{1, 2})
	if err == nil {
		t.Error("expected error for short bytes, got nil")
	}
}

func TestUint16ToBytes(t *testing.T) {
	b := Uint16ToBytes(0x1234)
	if len(b) != 2 {
		t.Fatalf("Uint16ToBytes length = %d, want 2", len(b))
	}
	if b[0] != 0x12 || b[1] != 0x34 {
		t.Errorf("Uint16ToBytes(0x1234) = %v", b)
	}
}

func TestBytesToUint16(t *testing.T) {
	got, err := BytesToUint16([]byte{0x12, 0x34})
	if err != nil {
		t.Fatalf("BytesToUint16 error: %v", err)
	}
	if got != 0x1234 {
		t.Errorf("BytesToUint16 = 0x%04X, want 0x1234", got)
	}
	_, err = BytesToUint16([]byte{1})
	if err == nil {
		t.Error("expected error for short bytes, got nil")
	}
}

func TestCIDRRoutesToBytes(t *testing.T) {
	routes := []CIDRRoute{
		{Destination: net.IPv4(10, 0, 1, 0), PrefixLen: 24, Gateway: net.IPv4(192, 168, 1, 1)},
	}
	b := CIDRRoutesToBytes(routes)
	// /24 → 1 byte prefix + 3 bytes significant + 4 bytes gateway = 8 bytes
	if len(b) != 8 {
		t.Fatalf("CIDRRoutesToBytes /24 length = %d, want 8", len(b))
	}
	if b[0] != 24 {
		t.Errorf("prefix length byte = %d, want 24", b[0])
	}

	// /0 default route
	routes2 := []CIDRRoute{
		{Destination: net.IPv4(0, 0, 0, 0), PrefixLen: 0, Gateway: net.IPv4(192, 168, 1, 1)},
	}
	b2 := CIDRRoutesToBytes(routes2)
	// /0 → 1 byte prefix + 0 bytes significant + 4 bytes gateway = 5 bytes
	if len(b2) != 5 {
		t.Fatalf("CIDRRoutesToBytes /0 length = %d, want 5", len(b2))
	}
	if b2[0] != 0 {
		t.Errorf("prefix length byte = %d, want 0", b2[0])
	}
}

func TestBytesToCIDRRoutes(t *testing.T) {
	// Encode two routes and decode them
	input := []CIDRRoute{
		{Destination: net.IPv4(10, 0, 1, 0), PrefixLen: 24, Gateway: net.IPv4(192, 168, 1, 1)},
		{Destination: net.IPv4(0, 0, 0, 0), PrefixLen: 0, Gateway: net.IPv4(192, 168, 1, 254)},
	}
	encoded := CIDRRoutesToBytes(input)
	routes, err := BytesToCIDRRoutes(encoded)
	if err != nil {
		t.Fatalf("BytesToCIDRRoutes error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("decoded %d routes, want 2", len(routes))
	}

	// Check first route
	if routes[0].PrefixLen != 24 {
		t.Errorf("route[0].PrefixLen = %d, want 24", routes[0].PrefixLen)
	}
	if !routes[0].Gateway.Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("route[0].Gateway = %s, want 192.168.1.1", routes[0].Gateway)
	}

	// Check second route (default)
	if routes[1].PrefixLen != 0 {
		t.Errorf("route[1].PrefixLen = %d, want 0", routes[1].PrefixLen)
	}
	if !routes[1].Gateway.Equal(net.IPv4(192, 168, 1, 254)) {
		t.Errorf("route[1].Gateway = %s, want 192.168.1.254", routes[1].Gateway)
	}
}

func TestBytesToCIDRRoutesInvalid(t *testing.T) {
	// Truncated data
	_, err := BytesToCIDRRoutes([]byte{24, 10, 0}) // Too short for /24 + gateway
	if err == nil {
		t.Error("expected error for truncated data, got nil")
	}

	// Empty data
	routes, err := BytesToCIDRRoutes([]byte{})
	if err != nil {
		t.Errorf("unexpected error for empty data: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("expected 0 routes for empty data, got %d", len(routes))
	}
}
