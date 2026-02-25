package ha

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func TestEncodeDecodeMessage(t *testing.T) {
	msg, err := NewHeartbeat("ACTIVE", 42, 100, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewHeartbeat error: %v", err)
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("EncodeMessage error: %v", err)
	}

	decoded, err := DecodeMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeMessage error: %v", err)
	}

	if decoded.Type != dhcpv4.HAMsgHeartbeat {
		t.Errorf("Type = %d, want %d", decoded.Type, dhcpv4.HAMsgHeartbeat)
	}
	if decoded.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp mismatch")
	}
}

func TestNewLeaseUpdate(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 100)
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	now := time.Now()

	msg, err := NewLeaseUpdate(ip, mac, "client1", "host1", "192.168.1.0/24", "pool1", "active", now, now.Add(time.Hour), 42)
	if err != nil {
		t.Fatalf("NewLeaseUpdate error: %v", err)
	}

	if msg.Type != dhcpv4.HAMsgLeaseUpdate {
		t.Errorf("Type = %d, want %d", msg.Type, dhcpv4.HAMsgLeaseUpdate)
	}

	// Roundtrip
	data, _ := EncodeMessage(msg)
	decoded, err := DecodeMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("roundtrip decode error: %v", err)
	}
	if decoded.Type != dhcpv4.HAMsgLeaseUpdate {
		t.Errorf("roundtrip type mismatch")
	}
}

func TestNewConflictUpdate(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 100)
	msg, err := NewConflictUpdate(ip, time.Now(), "arp_probe", "aa:bb:cc:dd:ee:ff", "192.168.1.0/24", 2, false)
	if err != nil {
		t.Fatalf("NewConflictUpdate error: %v", err)
	}
	if msg.Type != dhcpv4.HAMsgConflictUpdate {
		t.Errorf("Type = %d, want %d", msg.Type, dhcpv4.HAMsgConflictUpdate)
	}
}

func TestDecodeMessageTooLarge(t *testing.T) {
	// Craft a frame with a huge length
	buf := make([]byte, 4)
	buf[0] = 0xFF
	buf[1] = 0xFF
	buf[2] = 0xFF
	buf[3] = 0xFF

	_, err := DecodeMessage(bytes.NewReader(buf))
	if err == nil {
		t.Error("expected error for oversized message")
	}
}

func TestMultipleMessagesOnStream(t *testing.T) {
	var buf bytes.Buffer

	// Write 3 messages to the same stream
	for i := 0; i < 3; i++ {
		msg, _ := NewHeartbeat("ACTIVE", i, uint64(i), time.Duration(i)*time.Second)
		data, _ := EncodeMessage(msg)
		buf.Write(data)
	}

	// Read them back
	reader := bytes.NewReader(buf.Bytes())
	for i := 0; i < 3; i++ {
		msg, err := DecodeMessage(reader)
		if err != nil {
			t.Fatalf("message %d decode error: %v", i, err)
		}
		if msg.Type != dhcpv4.HAMsgHeartbeat {
			t.Errorf("message %d type = %d, want heartbeat", i, msg.Type)
		}
	}
}
