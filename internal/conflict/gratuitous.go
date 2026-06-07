package conflict

import (
	"log/slog"
	"net"
)

// SendGratuitousARP would broadcast a gratuitous ARP after DHCPACK to refresh
// neighbour ARP caches (RFC 2131 §4.4.1, optional). It requires raw frame access
// that is not implemented, so it is a no-op while the ARP prober is unavailable.
// Announcements are skipped rather than silently pretended.
func SendGratuitousARP(arpProber *ARPProber, clientMAC net.HardwareAddr, assignedIP net.IP, logger *slog.Logger) {
	if arpProber == nil || !arpProber.Available() {
		return
	}
	// Unreachable until native ARP framing lands; left as the wiring point.
	logger.Debug("gratuitous ARP requested but not implemented",
		"client_mac", clientMAC.String(),
		"assigned_ip", assignedIP.String())
}
