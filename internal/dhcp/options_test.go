package dhcp

import (
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func TestDecodeOptionsBasic(t *testing.T) {
	// Build options bytes: subnet mask + end
	data := []byte{
		byte(dhcpv4.OptionSubnetMask), 4, 255, 255, 255, 0,
		byte(dhcpv4.OptionEnd),
	}

	opts, err := DecodeOptions(data)
	if err != nil {
		t.Fatalf("DecodeOptions error: %v", err)
	}

	mask, ok := opts[dhcpv4.OptionSubnetMask]
	if !ok {
		t.Fatal("expected OptionSubnetMask in options")
	}
	if len(mask) != 4 || mask[0] != 255 || mask[1] != 255 || mask[2] != 255 || mask[3] != 0 {
		t.Errorf("subnet mask = %v, want [255 255 255 0]", mask)
	}
}

func TestDecodeOptionsMultiple(t *testing.T) {
	data := []byte{
		byte(dhcpv4.OptionDHCPMessageType), 1, byte(dhcpv4.MessageTypeDiscover),
		byte(dhcpv4.OptionHostname), 4, 't', 'e', 's', 't',
		byte(dhcpv4.OptionEnd),
	}

	opts, err := DecodeOptions(data)
	if err != nil {
		t.Fatalf("DecodeOptions error: %v", err)
	}

	if len(opts) != 2 {
		t.Errorf("expected 2 options, got %d", len(opts))
	}

	if mt, ok := opts[dhcpv4.OptionDHCPMessageType]; !ok || mt[0] != byte(dhcpv4.MessageTypeDiscover) {
		t.Errorf("message type wrong or missing")
	}

	if hn, ok := opts[dhcpv4.OptionHostname]; !ok || string(hn) != "test" {
		t.Errorf("hostname = %q, want %q", string(hn), "test")
	}
}

func TestDecodeOptionsPadding(t *testing.T) {
	data := []byte{
		byte(dhcpv4.OptionPad),
		byte(dhcpv4.OptionPad),
		byte(dhcpv4.OptionDHCPMessageType), 1, byte(dhcpv4.MessageTypeRequest),
		byte(dhcpv4.OptionPad),
		byte(dhcpv4.OptionEnd),
	}

	opts, err := DecodeOptions(data)
	if err != nil {
		t.Fatalf("DecodeOptions error: %v", err)
	}

	if len(opts) != 1 {
		t.Errorf("expected 1 option (pad should be skipped), got %d", len(opts))
	}
}

func TestDecodeOptionsTruncated(t *testing.T) {
	// Option with no length byte
	_, err := DecodeOptions([]byte{byte(dhcpv4.OptionSubnetMask)})
	if err == nil {
		t.Error("expected error for truncated option (no length byte)")
	}

	// Option with length but not enough data
	_, err = DecodeOptions([]byte{byte(dhcpv4.OptionSubnetMask), 4, 255, 255})
	if err == nil {
		t.Error("expected error for truncated option data")
	}
}

func TestOptionsEncode(t *testing.T) {
	opts := Options{
		dhcpv4.OptionDHCPMessageType: {byte(dhcpv4.MessageTypeOffer)},
		dhcpv4.OptionSubnetMask:      {255, 255, 255, 0},
	}

	encoded := opts.Encode()

	// Should end with End option
	if encoded[len(encoded)-1] != byte(dhcpv4.OptionEnd) {
		t.Error("encoded options should end with End option")
	}

	// Decode back and verify
	decoded, err := DecodeOptions(encoded)
	if err != nil {
		t.Fatalf("decode encoded options error: %v", err)
	}

	if mt, ok := decoded[dhcpv4.OptionDHCPMessageType]; !ok || mt[0] != byte(dhcpv4.MessageTypeOffer) {
		t.Error("message type not preserved in roundtrip")
	}
	if mask, ok := decoded[dhcpv4.OptionSubnetMask]; !ok || len(mask) != 4 {
		t.Error("subnet mask not preserved in roundtrip")
	}
}

func TestOptionsClone(t *testing.T) {
	opts := Options{
		dhcpv4.OptionSubnetMask: {255, 255, 255, 0},
	}
	clone := opts.Clone()

	// Modify original
	opts[dhcpv4.OptionSubnetMask][3] = 128

	// Clone should not be affected
	if clone[dhcpv4.OptionSubnetMask][3] != 0 {
		t.Error("clone was affected by modification to original")
	}
}

func TestOptionsSettersAndGetters(t *testing.T) {
	opts := make(Options)

	opts.SetUint32(dhcpv4.OptionIPLeaseTime, 3600)
	data, ok := opts.Get(dhcpv4.OptionIPLeaseTime)
	if !ok || len(data) != 4 {
		t.Fatal("SetUint32/Get failed")
	}

	opts.SetUint16(dhcpv4.OptionMaxDHCPMessageSize, 1500)
	if !opts.Has(dhcpv4.OptionMaxDHCPMessageSize) {
		t.Error("SetUint16/Has failed")
	}

	opts.SetString(dhcpv4.OptionHostname, "testhost")
	if data, ok := opts.Get(dhcpv4.OptionHostname); !ok || string(data) != "testhost" {
		t.Error("SetString/Get failed")
	}

	opts.SetBool(dhcpv4.OptionIPForwarding, true)
	if data, ok := opts.Get(dhcpv4.OptionIPForwarding); !ok || data[0] != 0x01 {
		t.Error("SetBool true failed")
	}

	opts.SetBool(dhcpv4.OptionIPForwarding, false)
	if data, ok := opts.Get(dhcpv4.OptionIPForwarding); !ok || data[0] != 0x00 {
		t.Error("SetBool false failed")
	}

	opts.Delete(dhcpv4.OptionIPForwarding)
	if opts.Has(dhcpv4.OptionIPForwarding) {
		t.Error("Delete failed — option still present")
	}
}
