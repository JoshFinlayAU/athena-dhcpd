package dhcp

import (
	"fmt"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Options is a map of DHCP option code to raw option data.
type Options map[dhcpv4.OptionCode][]byte

// DecodeOptions parses the options section of a DHCP packet.
// RFC 2132 — options are TLV (type-length-value) encoded.
func DecodeOptions(data []byte) (Options, error) {
	opts := make(Options)
	i := 0
	for i < len(data) {
		code := dhcpv4.OptionCode(data[i])
		i++

		// Pad option (RFC 2132 §3.1)
		if code == dhcpv4.OptionPad {
			continue
		}

		// End option (RFC 2132 §3.2)
		if code == dhcpv4.OptionEnd {
			break
		}

		// TLV: need at least 1 byte for length
		if i >= len(data) {
			return nil, fmt.Errorf("truncated option %d: no length byte", code)
		}

		length := int(data[i])
		i++

		if i+length > len(data) {
			return nil, fmt.Errorf("truncated option %d: need %d bytes, have %d", code, length, len(data)-i)
		}

		value := make([]byte, length)
		copy(value, data[i:i+length])
		opts[code] = value
		i += length
	}

	return opts, nil
}

// Encode serializes options to bytes with end marker.
func (opts Options) Encode() []byte {
	// Estimate size
	size := 0
	for _, v := range opts {
		size += 2 + len(v) // code + length + value
	}
	size++ // End option

	buf := make([]byte, 0, size)
	for code, value := range opts {
		if code == dhcpv4.OptionPad || code == dhcpv4.OptionEnd {
			continue
		}
		buf = append(buf, byte(code))
		buf = append(buf, byte(len(value)))
		buf = append(buf, value...)
	}

	// End option
	buf = append(buf, byte(dhcpv4.OptionEnd))
	return buf
}

// Get returns the raw value for an option code.
func (opts Options) Get(code dhcpv4.OptionCode) ([]byte, bool) {
	v, ok := opts[code]
	return v, ok
}

// Set sets an option to a raw value.
func (opts Options) Set(code dhcpv4.OptionCode, value []byte) {
	opts[code] = value
}

// SetIP sets an IP address option.
func (opts Options) SetIP(code dhcpv4.OptionCode, ip interface{}) {
	switch v := ip.(type) {
	case [4]byte:
		opts[code] = v[:]
	case []byte:
		opts[code] = v
	}
}

// SetUint32 sets a uint32 option.
func (opts Options) SetUint32(code dhcpv4.OptionCode, v uint32) {
	opts[code] = dhcpv4.Uint32ToBytes(v)
}

// SetUint16 sets a uint16 option.
func (opts Options) SetUint16(code dhcpv4.OptionCode, v uint16) {
	opts[code] = dhcpv4.Uint16ToBytes(v)
}

// SetString sets a string option.
func (opts Options) SetString(code dhcpv4.OptionCode, s string) {
	opts[code] = []byte(s)
}

// SetBool sets a boolean option (1 byte: 0x00 or 0x01).
func (opts Options) SetBool(code dhcpv4.OptionCode, v bool) {
	if v {
		opts[code] = []byte{0x01}
	} else {
		opts[code] = []byte{0x00}
	}
}

// Has returns true if the option is present.
func (opts Options) Has(code dhcpv4.OptionCode) bool {
	_, ok := opts[code]
	return ok
}

// Delete removes an option.
func (opts Options) Delete(code dhcpv4.OptionCode) {
	delete(opts, code)
}

// Clone returns a deep copy of the options.
func (opts Options) Clone() Options {
	clone := make(Options, len(opts))
	for k, v := range opts {
		vc := make([]byte, len(v))
		copy(vc, v)
		clone[k] = vc
	}
	return clone
}
