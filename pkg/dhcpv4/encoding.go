package dhcpv4

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// IPToBytes converts a net.IP to a 4-byte slice.
func IPToBytes(ip net.IP) []byte {
	ip4 := ip.To4()
	if ip4 == nil {
		return []byte{0, 0, 0, 0}
	}
	return []byte(ip4)
}

// BytesToIP converts a 4-byte slice to net.IP.
func BytesToIP(b []byte) net.IP {
	if len(b) != 4 {
		return nil
	}
	return net.IPv4(b[0], b[1], b[2], b[3])
}

// IPListToBytes converts a slice of net.IP to bytes (N*4).
func IPListToBytes(ips []net.IP) []byte {
	buf := make([]byte, 0, len(ips)*4)
	for _, ip := range ips {
		buf = append(buf, IPToBytes(ip)...)
	}
	return buf
}

// BytesToIPList converts bytes to a slice of net.IP (N*4).
func BytesToIPList(b []byte) ([]net.IP, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid IP list length %d: must be multiple of 4", len(b))
	}
	ips := make([]net.IP, 0, len(b)/4)
	for i := 0; i < len(b); i += 4 {
		ips = append(ips, BytesToIP(b[i:i+4]))
	}
	return ips, nil
}

// Uint16ToBytes converts a uint16 to 2 bytes (big-endian).
func Uint16ToBytes(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// BytesToUint16 converts 2 bytes to uint16 (big-endian).
func BytesToUint16(b []byte) (uint16, error) {
	if len(b) != 2 {
		return 0, fmt.Errorf("invalid uint16 length %d: expected 2", len(b))
	}
	return binary.BigEndian.Uint16(b), nil
}

// Uint32ToBytes converts a uint32 to 4 bytes (big-endian).
func Uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// BytesToUint32 converts 4 bytes to uint32 (big-endian).
func BytesToUint32(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, fmt.Errorf("invalid uint32 length %d: expected 4", len(b))
	}
	return binary.BigEndian.Uint32(b), nil
}

// Int32ToBytes converts an int32 to 4 bytes (big-endian).
func Int32ToBytes(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// BytesToInt32 converts 4 bytes to int32 (big-endian).
func BytesToInt32(b []byte) (int32, error) {
	if len(b) != 4 {
		return 0, fmt.Errorf("invalid int32 length %d: expected 4", len(b))
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

// MACToString formats a hardware address as a colon-separated string.
func MACToString(mac net.HardwareAddr) string {
	return mac.String()
}

// ParseMAC parses a colon-separated MAC address string.
func ParseMAC(s string) (net.HardwareAddr, error) {
	return net.ParseMAC(s)
}

// IPInSubnet checks if an IP is within a given subnet.
func IPInSubnet(ip net.IP, network *net.IPNet) bool {
	return network.Contains(ip)
}

// NextIP returns the next IP address after the given one.
func NextIP(ip net.IP) net.IP {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	next := make(net.IP, 4)
	copy(next, ip4)
	for i := 3; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

// PrevIP returns the previous IP address before the given one.
func PrevIP(ip net.IP) net.IP {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	prev := make(net.IP, 4)
	copy(prev, ip4)
	for i := 3; i >= 0; i-- {
		prev[i]--
		if prev[i] != 0xff {
			break
		}
	}
	return prev
}

// IPToUint32 converts a net.IP to a uint32.
func IPToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

// Uint32ToIP converts a uint32 to a net.IP.
func Uint32ToIP(n uint32) net.IP {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, n)
	return net.IPv4(b[0], b[1], b[2], b[3])
}

// IPRangeSize returns the number of IPs in a range (inclusive).
func IPRangeSize(start, end net.IP) uint32 {
	s := IPToUint32(start)
	e := IPToUint32(end)
	if e < s {
		return 0
	}
	return e - s + 1
}

// CIDRRoutesToBytes encodes classless static routes per RFC 3442.
// Each route is (prefix_len, significant_octets_of_subnet, gateway).
func CIDRRoutesToBytes(routes []CIDRRoute) []byte {
	var buf []byte
	for _, r := range routes {
		prefixLen := byte(r.PrefixLen)
		sigOctets := (r.PrefixLen + 7) / 8
		buf = append(buf, prefixLen)
		subnet := r.Destination.To4()
		if subnet == nil {
			continue
		}
		buf = append(buf, subnet[:sigOctets]...)
		buf = append(buf, IPToBytes(r.Gateway)...)
	}
	return buf
}

// BytesToCIDRRoutes decodes classless static routes per RFC 3442.
func BytesToCIDRRoutes(b []byte) ([]CIDRRoute, error) {
	var routes []CIDRRoute
	i := 0
	for i < len(b) {
		if i >= len(b) {
			break
		}
		prefixLen := int(b[i])
		i++
		if prefixLen > 32 {
			return nil, fmt.Errorf("invalid CIDR prefix length %d at offset %d", prefixLen, i-1)
		}
		sigOctets := (prefixLen + 7) / 8
		if i+sigOctets+4 > len(b) {
			return nil, fmt.Errorf("truncated CIDR route at offset %d", i)
		}
		dest := make([]byte, 4)
		copy(dest, b[i:i+sigOctets])
		i += sigOctets
		gateway := BytesToIP(b[i : i+4])
		i += 4

		mask := net.CIDRMask(prefixLen, 32)
		routes = append(routes, CIDRRoute{
			Destination: net.IP(dest).Mask(mask),
			PrefixLen:   prefixLen,
			Gateway:     gateway,
		})
	}
	return routes, nil
}

// CIDRRoute represents a classless static route (RFC 3442).
type CIDRRoute struct {
	Destination net.IP
	PrefixLen   int
	Gateway     net.IP
}

// FormatCIDRRoute returns a human-readable representation.
func (r CIDRRoute) String() string {
	return fmt.Sprintf("%s/%d via %s", r.Destination, r.PrefixLen, r.Gateway)
}

// ParseCIDR parses a CIDR string into network IP and mask.
func ParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}

// FormatMAC formats bytes as a MAC address string.
func FormatMAC(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, ":")
}
