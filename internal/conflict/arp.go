package conflict

import (
	"context"
	"fmt"
	"log/slog"
	"net"
)

// ARPProber would send ARP requests to detect IP conflicts on directly-attached
// subnets (RFC 826). Native ARP probing needs AF_PACKET (Linux) or BPF (BSD)
// raw frame access, which is not implemented here. The prober therefore always
// reports unavailable, so the detector falls back to ICMP echo probing — which
// works on local subnets too — instead of silently treating every address as
// clear.
type ARPProber struct {
	iface     *net.Interface
	srcIP     net.IP
	logger    *slog.Logger
	available bool
}

// NewARPProber resolves the interface and returns a prober. Because native ARP
// framing is not implemented, the prober is always unavailable and logs that
// ICMP will be used for local conflict detection.
func NewARPProber(ifaceName string, logger *slog.Logger) (*ARPProber, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("looking up interface %s: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("getting addresses for %s: %w", ifaceName, err)
	}

	var srcIP net.IP
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				srcIP = ip4
				break
			}
		}
	}
	if srcIP == nil {
		return nil, fmt.Errorf("no IPv4 address on interface %s", ifaceName)
	}

	logger.Warn("native ARP conflict probing is not implemented — using ICMP for local subnets",
		"interface", ifaceName)

	return &ARPProber{
		iface:     iface,
		srcIP:     srcIP,
		logger:    logger,
		available: false,
	}, nil
}

// Available reports whether ARP probing is usable. Always false until native ARP
// framing is implemented.
func (p *ARPProber) Available() bool {
	return p.available
}

// Close is a no-op; no socket is held.
func (p *ARPProber) Close() error {
	return nil
}

// Probe is a no-op in the unimplemented state and reports the address as clear.
// It is never reached while Available() is false (the detector routes to ICMP).
func (p *ARPProber) Probe(ctx context.Context, targetIP net.IP) (bool, string, error) {
	return false, "", nil
}

// Interface returns the network interface resolved for this prober.
func (p *ARPProber) Interface() *net.Interface {
	return p.iface
}

// SourceIP returns the interface IPv4 address.
func (p *ARPProber) SourceIP() net.IP {
	return p.srcIP
}
