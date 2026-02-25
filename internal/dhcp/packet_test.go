package dhcp

import (
	"net"
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// buildTestDiscover builds a minimal DHCPDISCOVER packet for testing.
func buildTestDiscover(mac net.HardwareAddr, xid uint32) []byte {
	pkt := make([]byte, 300)
	pkt[0] = byte(dhcpv4.OpCodeBootRequest)
	pkt[1] = byte(dhcpv4.HardwareTypeEthernet)
	pkt[2] = 6 // HLen
	pkt[3] = 0 // Hops

	// XID
	pkt[4] = byte(xid >> 24)
	pkt[5] = byte(xid >> 16)
	pkt[6] = byte(xid >> 8)
	pkt[7] = byte(xid)

	// CHAddr
	copy(pkt[28:34], mac)

	// Magic cookie
	copy(pkt[236:240], dhcpv4.MagicCookie)

	// Options: DHCP Message Type = DISCOVER
	pkt[240] = byte(dhcpv4.OptionDHCPMessageType)
	pkt[241] = 1
	pkt[242] = byte(dhcpv4.MessageTypeDiscover)
	pkt[243] = byte(dhcpv4.OptionEnd)

	return pkt
}

func TestDecodePacket(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	data := buildTestDiscover(mac, 0xDEADBEEF)

	pkt, err := DecodePacket(data)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	if pkt.Op != dhcpv4.OpCodeBootRequest {
		t.Errorf("Op = %d, want %d", pkt.Op, dhcpv4.OpCodeBootRequest)
	}
	if pkt.HType != dhcpv4.HardwareTypeEthernet {
		t.Errorf("HType = %d, want %d", pkt.HType, dhcpv4.HardwareTypeEthernet)
	}
	if pkt.HLen != 6 {
		t.Errorf("HLen = %d, want 6", pkt.HLen)
	}
	if pkt.XID != 0xDEADBEEF {
		t.Errorf("XID = 0x%08X, want 0xDEADBEEF", pkt.XID)
	}
	if pkt.CHAddr.String() != mac.String() {
		t.Errorf("CHAddr = %s, want %s", pkt.CHAddr, mac)
	}
	if pkt.MessageType() != dhcpv4.MessageTypeDiscover {
		t.Errorf("MessageType = %d, want DISCOVER(%d)", pkt.MessageType(), dhcpv4.MessageTypeDiscover)
	}
}

func TestDecodePacketTooShort(t *testing.T) {
	data := make([]byte, 100) // Too short
	_, err := DecodePacket(data)
	if err == nil {
		t.Error("expected error for short packet, got nil")
	}
}

func TestDecodePacketBadMagicCookie(t *testing.T) {
	data := make([]byte, 300)
	data[0] = 1 // Boot request
	data[1] = 1 // Ethernet
	data[2] = 6
	// Bad magic cookie at 236-239
	data[236] = 0xFF
	data[237] = 0xFF
	data[238] = 0xFF
	data[239] = 0xFF

	_, err := DecodePacket(data)
	if err == nil {
		t.Error("expected error for bad magic cookie, got nil")
	}
}

func TestPacketRoundTrip(t *testing.T) {
	mac := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	data := buildTestDiscover(mac, 0x12345678)

	pkt, err := DecodePacket(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	encoded, err := pkt.Encode()
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Decode again
	pkt2, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("re-decode error: %v", err)
	}

	if pkt2.XID != pkt.XID {
		t.Errorf("XID mismatch: 0x%08X vs 0x%08X", pkt2.XID, pkt.XID)
	}
	if pkt2.CHAddr.String() != pkt.CHAddr.String() {
		t.Errorf("CHAddr mismatch: %s vs %s", pkt2.CHAddr, pkt.CHAddr)
	}
	if pkt2.MessageType() != pkt.MessageType() {
		t.Errorf("MessageType mismatch: %d vs %d", pkt2.MessageType(), pkt.MessageType())
	}
}

func TestPacketMessageType(t *testing.T) {
	tests := []struct {
		name    string
		msgType dhcpv4.MessageType
	}{
		{"Discover", dhcpv4.MessageTypeDiscover},
		{"Offer", dhcpv4.MessageTypeOffer},
		{"Request", dhcpv4.MessageTypeRequest},
		{"Ack", dhcpv4.MessageTypeAck},
		{"Nak", dhcpv4.MessageTypeNak},
		{"Release", dhcpv4.MessageTypeRelease},
		{"Decline", dhcpv4.MessageTypeDecline},
		{"Inform", dhcpv4.MessageTypeInform},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{
				Options: Options{
					dhcpv4.OptionDHCPMessageType: {byte(tt.msgType)},
				},
			}
			if got := pkt.MessageType(); got != tt.msgType {
				t.Errorf("MessageType() = %d, want %d", got, tt.msgType)
			}
		})
	}
}

func TestPacketIsBroadcast(t *testing.T) {
	pkt := &Packet{Flags: 0x8000}
	if !pkt.IsBroadcast() {
		t.Error("expected IsBroadcast() = true")
	}
	pkt.Flags = 0x0000
	if pkt.IsBroadcast() {
		t.Error("expected IsBroadcast() = false")
	}
}

func TestPacketIsRelayed(t *testing.T) {
	pkt := &Packet{GIAddr: net.IPv4(192, 168, 1, 1)}
	if !pkt.IsRelayed() {
		t.Error("expected IsRelayed() = true")
	}
	pkt.GIAddr = net.IPv4zero
	if pkt.IsRelayed() {
		t.Error("expected IsRelayed() = false")
	}
}

func TestPacketNewReply(t *testing.T) {
	req := &Packet{
		Op:     dhcpv4.OpCodeBootRequest,
		HType:  dhcpv4.HardwareTypeEthernet,
		HLen:   6,
		XID:    0xCAFEBABE,
		Flags:  0x8000,
		GIAddr: net.IPv4(10, 0, 0, 1),
		CHAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Options: Options{
			dhcpv4.OptionDHCPMessageType: {byte(dhcpv4.MessageTypeDiscover)},
			dhcpv4.OptionClientIdentifier: {0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		},
	}

	serverIP := net.IPv4(192, 168, 1, 1)
	reply := req.NewReply(dhcpv4.MessageTypeOffer, serverIP)

	if reply.Op != dhcpv4.OpCodeBootReply {
		t.Errorf("Op = %d, want BOOTREPLY(%d)", reply.Op, dhcpv4.OpCodeBootReply)
	}
	if reply.XID != req.XID {
		t.Errorf("XID = 0x%08X, want 0x%08X", reply.XID, req.XID)
	}
	if reply.Flags != req.Flags {
		t.Errorf("Flags = 0x%04X, want 0x%04X", reply.Flags, req.Flags)
	}
	if reply.CHAddr.String() != req.CHAddr.String() {
		t.Errorf("CHAddr = %s, want %s", reply.CHAddr, req.CHAddr)
	}
	if !reply.GIAddr.Equal(req.GIAddr) {
		t.Errorf("GIAddr = %s, want %s", reply.GIAddr, req.GIAddr)
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("MessageType = %d, want OFFER", reply.MessageType())
	}

	// Server identifier should be set
	sid := reply.ServerIdentifier()
	if sid == nil || !sid.Equal(serverIP) {
		t.Errorf("ServerIdentifier = %s, want %s", sid, serverIP)
	}

	// Client identifier should be echoed (RFC 6842)
	cid := reply.ClientIdentifier()
	if cid == nil {
		t.Error("ClientIdentifier should be echoed in reply")
	}
}

func TestPacketRequestedIP(t *testing.T) {
	pkt := &Packet{
		Options: Options{
			dhcpv4.OptionRequestedIP: {192, 168, 1, 100},
		},
	}
	got := pkt.RequestedIP()
	if !got.Equal(net.IPv4(192, 168, 1, 100)) {
		t.Errorf("RequestedIP() = %s, want 192.168.1.100", got)
	}

	// No option set
	pkt2 := &Packet{Options: Options{}}
	if got := pkt2.RequestedIP(); got != nil {
		t.Errorf("RequestedIP() = %s, want nil", got)
	}
}

func TestPacketHostname(t *testing.T) {
	pkt := &Packet{
		Options: Options{
			dhcpv4.OptionHostname: []byte("myhost"),
		},
	}
	if got := pkt.Hostname(); got != "myhost" {
		t.Errorf("Hostname() = %q, want %q", got, "myhost")
	}
}

func TestPacketVendorClassID(t *testing.T) {
	pkt := &Packet{
		Options: Options{
			dhcpv4.OptionVendorClassID: []byte("MSFT 5.0"),
		},
	}
	if got := pkt.VendorClassID(); got != "MSFT 5.0" {
		t.Errorf("VendorClassID() = %q, want %q", got, "MSFT 5.0")
	}
}

func TestGetBufferPutBuffer(t *testing.T) {
	buf := GetBuffer()
	if len(buf) != dhcpv4.MaxPacketSize {
		t.Errorf("GetBuffer() length = %d, want %d", len(buf), dhcpv4.MaxPacketSize)
	}
	PutBuffer(buf) // Should not panic
}
