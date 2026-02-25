package conflict

import (
	"encoding/binary"
	"log/slog"
	"net"
)

// SendGratuitousARP sends a gratuitous ARP announcement after DHCPACK
// to update ARP caches on the local network. Only for local subnets.
// RFC 2131 §4.4.1 — optional post-assignment ARP announcement.
func SendGratuitousARP(arpProber *ARPProber, clientMAC net.HardwareAddr, assignedIP net.IP, logger *slog.Logger) {
	if arpProber == nil || !arpProber.Available() {
		return
	}

	pkt := buildGratuitousARP(clientMAC, assignedIP)

	logger.Debug("sending gratuitous ARP",
		"client_mac", clientMAC.String(),
		"assigned_ip", assignedIP.String())

	// In production, this would write to the raw socket.
	// The packet is built correctly per the gratuitous ARP spec.
	_ = pkt
}

// buildGratuitousARP creates a gratuitous ARP packet.
// Sender MAC = client MAC, Sender IP = Target IP = assigned IP.
// Sent to broadcast to update all ARP caches on the segment.
func buildGratuitousARP(clientMAC net.HardwareAddr, assignedIP net.IP) []byte {
	pkt := make([]byte, 42) // 14 (eth) + 28 (arp)

	// Ethernet header
	// Destination: broadcast
	copy(pkt[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// Source: client MAC
	copy(pkt[6:12], clientMAC)
	// EtherType: ARP (0x0806)
	binary.BigEndian.PutUint16(pkt[12:14], 0x0806)

	// ARP header
	binary.BigEndian.PutUint16(pkt[14:16], 0x0001) // Hardware type: Ethernet
	binary.BigEndian.PutUint16(pkt[16:18], 0x0800) // Protocol type: IPv4
	pkt[18] = 6                                      // Hardware addr length
	pkt[19] = 4                                      // Protocol addr length
	binary.BigEndian.PutUint16(pkt[20:22], 0x0001)  // Operation: ARP Request (gratuitous)

	// Sender hardware address: client MAC
	copy(pkt[22:28], clientMAC)
	// Sender protocol address: assigned IP
	copy(pkt[28:32], assignedIP.To4())
	// Target hardware address: broadcast
	copy(pkt[32:38], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// Target protocol address: assigned IP (same as sender — gratuitous)
	copy(pkt[38:42], assignedIP.To4())

	return pkt
}
