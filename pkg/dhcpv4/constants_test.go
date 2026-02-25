package dhcpv4

import "testing"

func TestMessageTypeString(t *testing.T) {
	tests := []struct {
		mt   MessageType
		want string
	}{
		{MessageTypeDiscover, "DHCPDISCOVER"},
		{MessageTypeOffer, "DHCPOFFER"},
		{MessageTypeRequest, "DHCPREQUEST"},
		{MessageTypeDecline, "DHCPDECLINE"},
		{MessageTypeAck, "DHCPACK"},
		{MessageTypeNak, "DHCPNAK"},
		{MessageTypeRelease, "DHCPRELEASE"},
		{MessageTypeInform, "DHCPINFORM"},
		{MessageType(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.mt.String(); got != tt.want {
			t.Errorf("MessageType(%d).String() = %q, want %q", tt.mt, got, tt.want)
		}
	}
}

func TestOptionCodeValues(t *testing.T) {
	// Verify key option codes match RFC 2132 values
	tests := []struct {
		code OptionCode
		want byte
	}{
		{OptionPad, 0},
		{OptionSubnetMask, 1},
		{OptionRouter, 3},
		{OptionDomainNameServer, 6},
		{OptionHostname, 12},
		{OptionDomainName, 15},
		{OptionRequestedIP, 50},
		{OptionIPLeaseTime, 51},
		{OptionDHCPMessageType, 53},
		{OptionServerIdentifier, 54},
		{OptionParameterRequestList, 55},
		{OptionRenewalTime, 58},
		{OptionRebindingTime, 59},
		{OptionClientIdentifier, 61},
		{OptionRelayAgentInfo, 82},
		{OptionClasslessStaticRoute, 121},
		{OptionEnd, 255},
	}
	for _, tt := range tests {
		if byte(tt.code) != tt.want {
			t.Errorf("OptionCode %d: got %d, want %d", tt.code, byte(tt.code), tt.want)
		}
	}
}

func TestLeaseStateValues(t *testing.T) {
	if LeaseStateActive != "active" {
		t.Errorf("LeaseStateActive = %q, want %q", LeaseStateActive, "active")
	}
	if LeaseStateOffered != "offered" {
		t.Errorf("LeaseStateOffered = %q, want %q", LeaseStateOffered, "offered")
	}
	if LeaseStateExpired != "expired" {
		t.Errorf("LeaseStateExpired = %q, want %q", LeaseStateExpired, "expired")
	}
}

func TestDetectionMethodValues(t *testing.T) {
	if DetectionARPProbe != "arp_probe" {
		t.Errorf("DetectionARPProbe = %q, want %q", DetectionARPProbe, "arp_probe")
	}
	if DetectionICMPProbe != "icmp_probe" {
		t.Errorf("DetectionICMPProbe = %q, want %q", DetectionICMPProbe, "icmp_probe")
	}
	if DetectionClientDecline != "client_decline" {
		t.Errorf("DetectionClientDecline = %q, want %q", DetectionClientDecline, "client_decline")
	}
}

func TestPacketSizeConstants(t *testing.T) {
	if MinPacketSize != 300 {
		t.Errorf("MinPacketSize = %d, want 300", MinPacketSize)
	}
	if MaxPacketSize != 1500 {
		t.Errorf("MaxPacketSize = %d, want 1500", MaxPacketSize)
	}
	if ServerPort != 67 {
		t.Errorf("ServerPort = %d, want 67", ServerPort)
	}
	if ClientPort != 68 {
		t.Errorf("ClientPort = %d, want 68", ClientPort)
	}
}

func TestMagicCookie(t *testing.T) {
	expected := []byte{99, 130, 83, 99}
	if len(MagicCookie) != 4 {
		t.Fatalf("MagicCookie length = %d, want 4", len(MagicCookie))
	}
	for i, b := range MagicCookie {
		if b != expected[i] {
			t.Errorf("MagicCookie[%d] = %d, want %d", i, b, expected[i])
		}
	}
}
