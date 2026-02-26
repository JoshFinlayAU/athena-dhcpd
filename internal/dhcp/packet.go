// Package dhcp implements the DHCPv4 server, packet handling, and option engine.
package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Packet represents a decoded DHCPv4 packet (RFC 2131 §2).
type Packet struct {
	Op      dhcpv4.OpCode       // Message op code: 1=BOOTREQUEST, 2=BOOTREPLY
	HType   dhcpv4.HardwareType // Hardware address type (1=Ethernet)
	HLen    byte                // Hardware address length (6 for Ethernet)
	Hops    byte                // Relay hops
	XID     uint32              // Transaction ID
	Secs    uint16              // Seconds elapsed
	Flags   uint16              // Flags (bit 0 = broadcast)
	CIAddr  net.IP              // Client IP address
	YIAddr  net.IP              // 'Your' (client) IP address
	SIAddr  net.IP              // Next server IP address
	GIAddr  net.IP              // Relay agent IP address
	CHAddr  net.HardwareAddr    // Client hardware address
	SName   [64]byte            // Server host name
	File    [128]byte           // Boot file name
	Options Options             // DHCP options

	// ReceivingInterface is set by the server to indicate which network
	// interface this packet arrived on. Not part of the wire format.
	ReceivingInterface string
}

// packetPool reuses packet buffers to reduce allocations in the hot path.
var packetPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, dhcpv4.MaxPacketSize)
	},
}

// GetBuffer returns a buffer from the pool.
func GetBuffer() []byte {
	return packetPool.Get().([]byte)
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(b []byte) {
	// Reset the buffer before returning
	for i := range b {
		b[i] = 0
	}
	packetPool.Put(b)
}

// DecodePacket parses a raw DHCPv4 packet from bytes.
// RFC 2131 §2 — packet format.
func DecodePacket(data []byte) (*Packet, error) {
	if len(data) < 240 {
		return nil, fmt.Errorf("packet too short: %d bytes (minimum 240)", len(data))
	}

	p := &Packet{}
	p.Op = dhcpv4.OpCode(data[0])
	p.HType = dhcpv4.HardwareType(data[1])
	p.HLen = data[2]
	p.Hops = data[3]
	p.XID = binary.BigEndian.Uint32(data[4:8])
	p.Secs = binary.BigEndian.Uint16(data[8:10])
	p.Flags = binary.BigEndian.Uint16(data[10:12])
	p.CIAddr = net.IP(make([]byte, 4))
	copy(p.CIAddr, data[12:16])
	p.YIAddr = net.IP(make([]byte, 4))
	copy(p.YIAddr, data[16:20])
	p.SIAddr = net.IP(make([]byte, 4))
	copy(p.SIAddr, data[20:24])
	p.GIAddr = net.IP(make([]byte, 4))
	copy(p.GIAddr, data[24:28])

	// Client hardware address (16 bytes in header, but only HLen are significant)
	chaddr := make([]byte, 16)
	copy(chaddr, data[28:44])
	if p.HLen <= 16 {
		p.CHAddr = net.HardwareAddr(chaddr[:p.HLen])
	} else {
		p.CHAddr = net.HardwareAddr(chaddr[:6])
	}

	copy(p.SName[:], data[44:108])
	copy(p.File[:], data[108:236])

	// Validate magic cookie (RFC 2131 §3)
	if len(data) >= 240 {
		cookie := data[236:240]
		if cookie[0] != 99 || cookie[1] != 130 || cookie[2] != 83 || cookie[3] != 99 {
			return nil, fmt.Errorf("invalid DHCP magic cookie: %v", cookie)
		}
	}

	// Parse options
	if len(data) > 240 {
		opts, err := DecodeOptions(data[240:])
		if err != nil {
			return nil, fmt.Errorf("decoding options: %w", err)
		}
		p.Options = opts
	} else {
		p.Options = make(Options)
	}

	return p, nil
}

// Encode serializes a DHCPv4 packet to bytes.
func (p *Packet) Encode() ([]byte, error) {
	// Fixed header: 236 bytes + 4 magic cookie + options
	optBytes := p.Options.Encode()
	totalLen := 240 + len(optBytes)
	if totalLen < dhcpv4.MinPacketSize {
		totalLen = dhcpv4.MinPacketSize
	}

	buf := make([]byte, totalLen)
	buf[0] = byte(p.Op)
	buf[1] = byte(p.HType)
	buf[2] = p.HLen
	buf[3] = p.Hops
	binary.BigEndian.PutUint32(buf[4:8], p.XID)
	binary.BigEndian.PutUint16(buf[8:10], p.Secs)
	binary.BigEndian.PutUint16(buf[10:12], p.Flags)

	if p.CIAddr != nil {
		copy(buf[12:16], p.CIAddr.To4())
	}
	if p.YIAddr != nil {
		copy(buf[16:20], p.YIAddr.To4())
	}
	if p.SIAddr != nil {
		copy(buf[20:24], p.SIAddr.To4())
	}
	if p.GIAddr != nil {
		copy(buf[24:28], p.GIAddr.To4())
	}
	if p.CHAddr != nil {
		copy(buf[28:44], p.CHAddr)
	}
	copy(buf[44:108], p.SName[:])
	copy(buf[108:236], p.File[:])

	// Magic cookie
	copy(buf[236:240], dhcpv4.MagicCookie)

	// Options
	copy(buf[240:], optBytes)

	return buf, nil
}

// MessageType returns the DHCP message type from the packet options.
func (p *Packet) MessageType() dhcpv4.MessageType {
	if data, ok := p.Options[dhcpv4.OptionDHCPMessageType]; ok && len(data) == 1 {
		return dhcpv4.MessageType(data[0])
	}
	return 0
}

// RequestedIP returns the requested IP address from option 50.
func (p *Packet) RequestedIP() net.IP {
	if data, ok := p.Options[dhcpv4.OptionRequestedIP]; ok && len(data) == 4 {
		return net.IP(data)
	}
	return nil
}

// ServerIdentifier returns the server identifier from option 54.
func (p *Packet) ServerIdentifier() net.IP {
	if data, ok := p.Options[dhcpv4.OptionServerIdentifier]; ok && len(data) == 4 {
		return net.IP(data)
	}
	return nil
}

// ClientIdentifier returns the client identifier from option 61.
func (p *Packet) ClientIdentifier() []byte {
	if data, ok := p.Options[dhcpv4.OptionClientIdentifier]; ok {
		return data
	}
	return nil
}

// Hostname returns the hostname from option 12.
func (p *Packet) Hostname() string {
	if data, ok := p.Options[dhcpv4.OptionHostname]; ok {
		return string(data)
	}
	return ""
}

// ParameterRequestList returns the list of requested option codes.
func (p *Packet) ParameterRequestList() []dhcpv4.OptionCode {
	if data, ok := p.Options[dhcpv4.OptionParameterRequestList]; ok {
		codes := make([]dhcpv4.OptionCode, len(data))
		for i, b := range data {
			codes[i] = dhcpv4.OptionCode(b)
		}
		return codes
	}
	return nil
}

// IsBroadcast returns true if the broadcast flag is set.
func (p *Packet) IsBroadcast() bool {
	return p.Flags&0x8000 != 0
}

// IsRelayed returns true if the packet was relayed (GIAddr is non-zero).
func (p *Packet) IsRelayed() bool {
	return p.GIAddr != nil && !p.GIAddr.Equal(net.IPv4zero)
}

// NewReply creates a response packet from a request, with common fields pre-filled.
func (p *Packet) NewReply(msgType dhcpv4.MessageType, serverIP net.IP) *Packet {
	reply := &Packet{
		Op:      dhcpv4.OpCodeBootReply,
		HType:   p.HType,
		HLen:    p.HLen,
		Hops:    0,
		XID:     p.XID,
		Secs:    0,
		Flags:   p.Flags,
		CIAddr:  net.IPv4zero,
		YIAddr:  net.IPv4zero,
		SIAddr:  serverIP,
		GIAddr:  make(net.IP, 4),
		CHAddr:  make(net.HardwareAddr, len(p.CHAddr)),
		Options: make(Options),
	}
	if gi := p.GIAddr.To4(); gi != nil {
		copy(reply.GIAddr, gi)
	} else {
		copy(reply.GIAddr, p.GIAddr)
	}
	copy(reply.CHAddr, p.CHAddr)

	// RFC 2131 §4.3.1 — set message type
	reply.Options[dhcpv4.OptionDHCPMessageType] = []byte{byte(msgType)}
	// RFC 2131 §4.3.1 — set server identifier
	reply.Options[dhcpv4.OptionServerIdentifier] = dhcpv4.IPToBytes(serverIP)

	// RFC 6842 — echo client-id back in responses
	if clientID := p.ClientIdentifier(); clientID != nil {
		reply.Options[dhcpv4.OptionClientIdentifier] = clientID
	}

	return reply
}

// VendorClassID returns the vendor class identifier from option 60.
func (p *Packet) VendorClassID() string {
	if data, ok := p.Options[dhcpv4.OptionVendorClassID]; ok {
		return string(data)
	}
	return ""
}

// UserClassID returns the user class identifier from option 77 (RFC 3004).
func (p *Packet) UserClassID() string {
	if data, ok := p.Options[dhcpv4.OptionUserClass]; ok {
		return string(data)
	}
	return ""
}

// MaxMessageSize returns the maximum DHCP message size from option 57.
func (p *Packet) MaxMessageSize() uint16 {
	if data, ok := p.Options[dhcpv4.OptionMaxDHCPMessageSize]; ok && len(data) == 2 {
		return binary.BigEndian.Uint16(data)
	}
	return 0
}
