package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// HAChecker is satisfied by the HA FSM — lets the handler skip packets when standby.
type HAChecker interface {
	IsActive() bool
}

// Handler processes DHCP messages implementing the DORA cycle (RFC 2131).
type Handler struct {
	cfg      *config.Config
	leases   *lease.Manager
	pools    map[string][]*pool.Pool // subnet network string → pools
	detector *conflict.Detector
	bus      *events.Bus
	logger   *slog.Logger
	serverIP net.IP
	ifaceIP  net.IP // auto-discovered from listening interface
	ha       HAChecker
}

// NewHandler creates a new DHCP message handler.
func NewHandler(
	cfg *config.Config,
	leases *lease.Manager,
	pools map[string][]*pool.Pool,
	detector *conflict.Detector,
	bus *events.Bus,
	logger *slog.Logger,
) *Handler {
	h := &Handler{
		cfg:      cfg,
		leases:   leases,
		pools:    pools,
		detector: detector,
		bus:      bus,
		logger:   logger,
		serverIP: cfg.ServerIP(),
	}

	// Auto-discover interface IP for subnet matching fallback
	if iface, err := net.InterfaceByName(cfg.Server.Interface); err == nil {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && ip.To4() != nil {
					h.ifaceIP = ip.To4()
					break
				}
			}
		}
	}
	if h.ifaceIP != nil {
		logger.Info("interface IP discovered for subnet matching", "interface", cfg.Server.Interface, "ip", h.ifaceIP.String())
	}

	return h
}

// SetHA sets the HA state checker (call after FSM is created).
func (h *Handler) SetHA(ha HAChecker) {
	h.ha = ha
}

// UpdateDetector sets or replaces the conflict detector (used by secondary on failover).
func (h *Handler) UpdateDetector(d *conflict.Detector) {
	h.detector = d
}

// HandlePacket dispatches a DHCP packet to the appropriate handler based on message type.
func (h *Handler) HandlePacket(ctx context.Context, pkt *Packet, src net.Addr) (*Packet, error) {
	// HA guard: if we have an FSM and we are NOT the active node, silently drop.
	if h.ha != nil && !h.ha.IsActive() {
		return nil, nil
	}

	msgType := pkt.MessageType()

	h.logger.Debug("received DHCP packet",
		"msg_type", msgType.String(),
		"mac", pkt.CHAddr.String(),
		"xid", fmt.Sprintf("%08x", pkt.XID),
		"ciaddr", pkt.CIAddr.String(),
		"giaddr", pkt.GIAddr.String())

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		return h.handleDiscover(ctx, pkt)
	case dhcpv4.MessageTypeRequest:
		return h.handleRequest(ctx, pkt)
	case dhcpv4.MessageTypeDecline:
		h.handleDecline(pkt)
		return nil, nil
	case dhcpv4.MessageTypeRelease:
		h.handleRelease(pkt)
		return nil, nil
	case dhcpv4.MessageTypeInform:
		return h.handleInform(pkt)
	default:
		h.logger.Warn("unsupported DHCP message type",
			"msg_type", msgType.String(),
			"mac", pkt.CHAddr.String())
		return nil, nil
	}
}

// handleDiscover processes DHCPDISCOVER → DHCPOFFER.
// RFC 2131 §4.3.1 — server response to DHCPDISCOVER.
func (h *Handler) handleDiscover(ctx context.Context, pkt *Packet) (*Packet, error) {
	mac := pkt.CHAddr
	clientID := fmt.Sprintf("%x", pkt.ClientIdentifier())
	hostname := pkt.Hostname()

	h.logger.Info("DHCPDISCOVER",
		"mac", mac.String(),
		"hostname", hostname,
		"requested_ip", pkt.RequestedIP())

	// Fire discover event
	h.bus.Publish(events.Event{
		Type:      events.EventLeaseDiscover,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			MAC:      mac.String(),
			Hostname: hostname,
		},
	})

	// Find the subnet for this request
	subnetIdx, subnetCfg := h.findSubnet(pkt)
	if subnetIdx < 0 {
		h.logger.Warn("no matching subnet for DISCOVER",
			"mac", mac.String(),
			"giaddr", pkt.GIAddr.String())
		return nil, nil // Silently ignore — no subnet to serve
	}

	// Check for reservation
	res := h.leases.FindReservation(clientID, mac, subnetIdx)
	if res != nil {
		ip := net.ParseIP(res.IP)
		// Validate reservation IP is within the subnet CIDR
		_, network, _ := net.ParseCIDR(subnetCfg.Network)
		if ip != nil && network != nil && network.Contains(ip) {
			return h.buildOffer(ctx, pkt, ip, mac, clientID, hostname, subnetIdx, subnetCfg, "", true)
		}
		h.logger.Warn("reservation IP outside subnet CIDR, skipping",
			"mac", mac.String(),
			"reservation_ip", res.IP,
			"subnet", subnetCfg.Network)
	}

	// Check for existing lease
	existing := h.leases.FindExistingLease(clientID, mac)
	if existing != nil && existing.Subnet == subnetCfg.Network {
		// Re-offer the same IP
		return h.buildOffer(ctx, pkt, existing.IP, mac, clientID, hostname, subnetIdx, subnetCfg, existing.Pool, false)
	}

	// Check if client requested a specific IP
	requestedIP := pkt.RequestedIP()

	// Find matching pool
	criteria := pool.MatchCriteria{
		VendorClass: pkt.VendorClassID(),
		UserClass:   pkt.UserClassID(),
	}
	relayInfo := GetRelayInfo(pkt)
	if relayInfo != nil {
		criteria.CircuitID = relayInfo.CircuitID
		criteria.RemoteID = relayInfo.RemoteID
	}

	subnetPools := h.pools[subnetCfg.Network]
	selectedPool := pool.SelectPool(subnetPools, criteria)
	if selectedPool == nil {
		h.logger.Warn("no matching pool for DISCOVER",
			"mac", mac.String(),
			"subnet", subnetCfg.Network)
		return nil, nil
	}

	// Try requested IP first if valid
	if requestedIP != nil && selectedPool.Contains(requestedIP) && !selectedPool.IsAllocated(requestedIP) {
		return h.buildOffer(ctx, pkt, requestedIP, mac, clientID, hostname, subnetIdx, subnetCfg, selectedPool.RangeString(), false)
	}

	// Allocate from pool — get candidates for conflict probing
	if h.detector != nil && h.cfg.ConflictDetection.Enabled {
		candidates := selectedPool.AllocateN(h.cfg.ConflictDetection.MaxProbesPerDiscover)
		if len(candidates) == 0 {
			metrics.PoolExhausted.WithLabelValues(subnetCfg.Network).Inc()
			h.logger.Warn("pool exhausted",
				"subnet", subnetCfg.Network,
				"pool", selectedPool.String())
			return nil, nil
		}

		// Probe candidates — RFC 2131 §4.4.1
		clearIP, err := h.detector.ProbeAndSelect(ctx, candidates, subnetCfg.Network)
		if err != nil {
			h.logger.Warn("all candidate IPs conflicted",
				"subnet", subnetCfg.Network,
				"error", err)
			return nil, nil
		}

		// Mark the selected IP as allocated
		selectedPool.AllocateSpecific(clearIP)
		return h.buildOffer(ctx, pkt, clearIP, mac, clientID, hostname, subnetIdx, subnetCfg, selectedPool.RangeString(), false)
	}

	// No conflict detection — just allocate
	ip := selectedPool.Allocate()
	if ip == nil {
		metrics.PoolExhausted.WithLabelValues(subnetCfg.Network).Inc()
		h.logger.Warn("pool exhausted",
			"subnet", subnetCfg.Network,
			"pool", selectedPool.String())
		return nil, nil
	}

	return h.buildOffer(ctx, pkt, ip, mac, clientID, hostname, subnetIdx, subnetCfg, selectedPool.RangeString(), false)
}

// buildOffer constructs and sends a DHCPOFFER.
func (h *Handler) buildOffer(ctx context.Context, pkt *Packet, ip net.IP, mac net.HardwareAddr,
	clientID, hostname string, subnetIdx int, subnetCfg *config.SubnetConfig, poolRange string, isReservation bool) (*Packet, error) {

	leaseTime := h.cfg.GetLeaseTime(subnetIdx)

	// Create the offer in the lease manager
	var relayInfo *lease.RelayInfo
	if pkt.IsRelayed() {
		ri := GetRelayInfo(pkt)
		if ri != nil {
			relayInfo = &lease.RelayInfo{
				GIAddr:    pkt.GIAddr,
				CircuitID: ri.CircuitID,
				RemoteID:  ri.RemoteID,
			}
		}
	}

	_, err := h.leases.CreateOffer(ip, mac, clientID, hostname, subnetCfg.Network, poolRange, leaseTime, relayInfo)
	if err != nil {
		return nil, fmt.Errorf("creating offer for %s: %w", mac, err)
	}

	// Build DHCPOFFER packet
	reply := pkt.NewReply(dhcpv4.MessageTypeOffer, h.serverIP)
	reply.YIAddr = ip

	// Set options from config
	h.setSubnetOptions(reply, subnetIdx, subnetCfg, leaseTime)

	// Copy relay agent info back (RFC 3046)
	if pkt.Options.Has(dhcpv4.OptionRelayAgentInfo) {
		reply.Options[dhcpv4.OptionRelayAgentInfo] = pkt.Options[dhcpv4.OptionRelayAgentInfo]
	}

	return reply, nil
}

// handleRequest processes DHCPREQUEST → DHCPACK or DHCPNAK.
// RFC 2131 §4.3.2 — server response to DHCPREQUEST.
func (h *Handler) handleRequest(ctx context.Context, pkt *Packet) (*Packet, error) {
	mac := pkt.CHAddr
	clientID := fmt.Sprintf("%x", pkt.ClientIdentifier())
	hostname := pkt.Hostname()
	requestedIP := pkt.RequestedIP()
	serverID := pkt.ServerIdentifier()

	h.logger.Info("DHCPREQUEST",
		"mac", mac.String(),
		"requested_ip", requestedIP,
		"server_id", serverID,
		"ciaddr", pkt.CIAddr.String())

	// If request has server-id and it's not us, ignore (RFC 2131 §4.3.2)
	if serverID != nil && !serverID.Equal(h.serverIP) {
		h.logger.Debug("DHCPREQUEST not for us, ignoring",
			"mac", mac.String(),
			"server_id", serverID.String())
		return nil, nil
	}

	// Determine the IP being requested
	var ip net.IP
	if requestedIP != nil {
		ip = requestedIP
	} else if !pkt.CIAddr.Equal(net.IPv4zero) {
		ip = pkt.CIAddr // Renewal
	}

	if ip == nil {
		return h.buildNAK(pkt, "no IP address in request"), nil
	}

	// Find subnet
	subnetIdx, subnetCfg := h.findSubnet(pkt)
	if subnetIdx < 0 {
		return h.buildNAK(pkt, "no matching subnet"), nil
	}

	// Verify the requested IP is within the subnet CIDR
	_, subnetNet, _ := net.ParseCIDR(subnetCfg.Network)
	if subnetNet != nil && !subnetNet.Contains(ip) {
		h.logger.Warn("DHCPREQUEST IP outside subnet CIDR",
			"mac", mac.String(),
			"requested_ip", ip.String(),
			"subnet", subnetCfg.Network)
		return h.buildNAK(pkt, "requested IP not in subnet"), nil
	}

	// Verify the IP is valid for this client
	existing := h.leases.FindExistingLease(clientID, mac)
	if existing != nil && !existing.IP.Equal(ip) {
		// Client is requesting a different IP than what was offered
		h.logger.Warn("DHCPREQUEST IP mismatch",
			"mac", mac.String(),
			"requested", ip.String(),
			"offered", existing.IP.String())
	}

	leaseTime := h.cfg.GetLeaseTime(subnetIdx)

	var relayInfo *lease.RelayInfo
	if pkt.IsRelayed() {
		ri := GetRelayInfo(pkt)
		if ri != nil {
			relayInfo = &lease.RelayInfo{
				GIAddr:    pkt.GIAddr,
				CircuitID: ri.CircuitID,
				RemoteID:  ri.RemoteID,
			}
		}
	}

	// Confirm the lease
	poolRange := ""
	if existing != nil {
		poolRange = existing.Pool
	}
	_, err := h.leases.ConfirmLease(ip, mac, clientID, hostname, subnetCfg.Network, poolRange, leaseTime, relayInfo)
	if err != nil {
		return nil, fmt.Errorf("confirming lease for %s: %w", mac, err)
	}

	// Send gratuitous ARP after successful ACK (local subnets only)
	if h.detector != nil && h.cfg.ConflictDetection.SendGratuitousARP {
		h.detector.SendGratuitousARPForLease(mac, ip)
	}

	// Build DHCPACK
	reply := pkt.NewReply(dhcpv4.MessageTypeAck, h.serverIP)
	reply.YIAddr = ip

	// For renewal, set CIAddr
	if !pkt.CIAddr.Equal(net.IPv4zero) {
		reply.CIAddr = pkt.CIAddr
	}

	h.setSubnetOptions(reply, subnetIdx, subnetCfg, leaseTime)

	// Copy relay agent info back
	if pkt.Options.Has(dhcpv4.OptionRelayAgentInfo) {
		reply.Options[dhcpv4.OptionRelayAgentInfo] = pkt.Options[dhcpv4.OptionRelayAgentInfo]
	}

	return reply, nil
}

// handleDecline processes DHCPDECLINE — client detected IP conflict.
// RFC 2131 §3.1 — client sends DECLINE when it detects the offered IP is in use.
func (h *Handler) handleDecline(pkt *Packet) {
	mac := pkt.CHAddr
	requestedIP := pkt.RequestedIP()

	if requestedIP == nil {
		h.logger.Warn("DHCPDECLINE without requested IP",
			"mac", mac.String())
		return
	}

	h.logger.Warn("DHCPDECLINE",
		"mac", mac.String(),
		"ip", requestedIP.String())

	// Let the lease manager handle it
	if err := h.leases.Decline(requestedIP, mac); err != nil {
		h.logger.Error("failed to process DECLINE",
			"ip", requestedIP.String(),
			"error", err)
	}

	// Let the conflict detector handle it
	if h.detector != nil {
		subnetIdx, subnetCfg := h.findSubnetForIP(requestedIP)
		subnet := ""
		if subnetIdx >= 0 {
			subnet = subnetCfg.Network
		}
		h.detector.HandleDecline(requestedIP, mac, subnet)
	}

	// Release the IP from the pool
	for _, subnetPools := range h.pools {
		for _, p := range subnetPools {
			if p.Contains(requestedIP) {
				p.Release(requestedIP)
				break
			}
		}
	}
}

// handleRelease processes DHCPRELEASE — client voluntarily releasing its lease.
// RFC 2131 §4.4.4 — client sends RELEASE when done with the IP.
func (h *Handler) handleRelease(pkt *Packet) {
	mac := pkt.CHAddr
	ip := pkt.CIAddr

	h.logger.Info("DHCPRELEASE",
		"mac", mac.String(),
		"ip", ip.String())

	if err := h.leases.Release(ip, mac); err != nil {
		h.logger.Error("failed to process RELEASE",
			"ip", ip.String(),
			"error", err)
	}

	// Release IP back to pool
	for _, subnetPools := range h.pools {
		for _, p := range subnetPools {
			if p.Contains(ip) {
				p.Release(ip)
				break
			}
		}
	}
}

// handleInform processes DHCPINFORM — client requesting options only (no IP assignment).
// RFC 2131 §4.3.5 — server responds with DHCPACK containing options only.
func (h *Handler) handleInform(pkt *Packet) (*Packet, error) {
	mac := pkt.CHAddr

	h.logger.Info("DHCPINFORM",
		"mac", mac.String(),
		"ciaddr", pkt.CIAddr.String())

	subnetIdx, subnetCfg := h.findSubnet(pkt)
	if subnetIdx < 0 {
		return nil, nil
	}

	reply := pkt.NewReply(dhcpv4.MessageTypeAck, h.serverIP)
	reply.CIAddr = pkt.CIAddr
	// YIAddr MUST be 0 for INFORM responses (RFC 2131 §4.3.5)
	reply.YIAddr = net.IPv4zero

	// Set options (no lease time for INFORM)
	h.setSubnetOptions(reply, subnetIdx, subnetCfg, 0)
	reply.Options.Delete(dhcpv4.OptionIPLeaseTime)
	reply.Options.Delete(dhcpv4.OptionRenewalTime)
	reply.Options.Delete(dhcpv4.OptionRebindingTime)

	return reply, nil
}

// buildNAK creates a DHCPNAK response.
func (h *Handler) buildNAK(pkt *Packet, reason string) *Packet {
	h.logger.Warn("DHCPNAK",
		"mac", pkt.CHAddr.String(),
		"reason", reason)

	h.bus.Publish(events.Event{
		Type:      events.EventLeaseNak,
		Timestamp: time.Now(),
		Lease: &events.LeaseData{
			MAC: pkt.CHAddr.String(),
		},
		Reason: reason,
	})

	reply := pkt.NewReply(dhcpv4.MessageTypeNak, h.serverIP)
	if reason != "" {
		reply.Options.SetString(dhcpv4.OptionMessage, reason)
	}
	return reply
}

// findSubnet determines which subnet a request belongs to.
// Uses giaddr for relayed packets, interface matching for direct packets.
func (h *Handler) findSubnet(pkt *Packet) (int, *config.SubnetConfig) {
	// Check for subnet selection option (RFC 3011, option 118)
	if subSelData, ok := pkt.Options[dhcpv4.OptionSubnetSelection]; ok && len(subSelData) == 4 {
		subSelIP := net.IP(subSelData)
		return h.findSubnetForIP(subSelIP)
	}

	// Check relay agent link selection (RFC 3527, option 82 sub 5)
	relayInfo := GetRelayInfo(pkt)
	if relayInfo != nil && len(relayInfo.LinkSelect) == 4 {
		linkIP := net.IP(relayInfo.LinkSelect)
		return h.findSubnetForIP(linkIP)
	}

	// Relayed: use giaddr
	if pkt.IsRelayed() {
		return h.findSubnetForIP(pkt.GIAddr)
	}

	// Direct: ciaddr set means renewal/rebind — use client's current IP
	// RFC 2131 §4.3.2: renewing client sets ciaddr to its current address
	if !pkt.CIAddr.Equal(net.IPv4zero) {
		return h.findSubnetForIP(pkt.CIAddr)
	}

	// Direct: match by receiving interface — each subnet declares its interface
	if pkt.ReceivingInterface != "" {
		if idx, sub := h.findSubnetForInterface(pkt.ReceivingInterface); idx >= 0 {
			return idx, sub
		}
	}

	// Fallback: use server IP if configured
	if h.serverIP != nil {
		return h.findSubnetForIP(h.serverIP)
	}

	// Fallback: discover IP from the default interface
	if h.ifaceIP != nil {
		return h.findSubnetForIP(h.ifaceIP)
	}

	return -1, nil
}

// findSubnetForInterface finds the subnet assigned to a specific network interface.
func (h *Handler) findSubnetForInterface(iface string) (int, *config.SubnetConfig) {
	for i, sub := range h.cfg.Subnets {
		if sub.Interface == iface {
			return i, &h.cfg.Subnets[i]
		}
	}
	return -1, nil
}

// findSubnetForIP finds the subnet config that contains the given IP.
func (h *Handler) findSubnetForIP(ip net.IP) (int, *config.SubnetConfig) {
	for i, sub := range h.cfg.Subnets {
		_, network, err := net.ParseCIDR(sub.Network)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return i, &h.cfg.Subnets[i]
		}
	}
	return -1, nil
}

// setSubnetOptions populates response options from subnet configuration.
func (h *Handler) setSubnetOptions(reply *Packet, subnetIdx int, subnetCfg *config.SubnetConfig, leaseTime time.Duration) {
	_, network, _ := net.ParseCIDR(subnetCfg.Network)

	// Subnet mask
	reply.Options[dhcpv4.OptionSubnetMask] = []byte(network.Mask)

	// Routers
	if len(subnetCfg.Routers) > 0 {
		var routers []net.IP
		for _, r := range subnetCfg.Routers {
			if ip := net.ParseIP(r); ip != nil {
				routers = append(routers, ip)
			}
		}
		if len(routers) > 0 {
			reply.Options[dhcpv4.OptionRouter] = dhcpv4.IPListToBytes(routers)
		}
	}

	// DNS servers (subnet → defaults)
	dnsServers := subnetCfg.DNSServers
	if len(dnsServers) == 0 {
		dnsServers = h.cfg.Defaults.DNSServers
	}
	if len(dnsServers) > 0 {
		var ips []net.IP
		for _, s := range dnsServers {
			if ip := net.ParseIP(s); ip != nil {
				ips = append(ips, ip)
			}
		}
		if len(ips) > 0 {
			reply.Options[dhcpv4.OptionDomainNameServer] = dhcpv4.IPListToBytes(ips)
		}
	}

	// Domain name
	domainName := subnetCfg.DomainName
	if domainName == "" {
		domainName = h.cfg.Defaults.DomainName
	}
	if domainName != "" {
		reply.Options.SetString(dhcpv4.OptionDomainName, domainName)
	}

	// NTP servers
	if len(subnetCfg.NTPServers) > 0 {
		var ips []net.IP
		for _, s := range subnetCfg.NTPServers {
			if ip := net.ParseIP(s); ip != nil {
				ips = append(ips, ip)
			}
		}
		if len(ips) > 0 {
			reply.Options[dhcpv4.OptionNTPServers] = dhcpv4.IPListToBytes(ips)
		}
	}

	// Broadcast address
	broadcastIP := dhcpv4.Uint32ToIP(dhcpv4.IPToUint32(network.IP) | ^dhcpv4.IPToUint32(net.IP(network.Mask)))
	reply.Options[dhcpv4.OptionBroadcastAddress] = dhcpv4.IPToBytes(broadcastIP)

	// Lease timing
	if leaseTime > 0 {
		reply.Options.SetUint32(dhcpv4.OptionIPLeaseTime, uint32(leaseTime.Seconds()))
		renewalTime := h.cfg.GetRenewalTime(subnetIdx)
		rebindTime := h.cfg.GetRebindTime(subnetIdx)
		reply.Options.SetUint32(dhcpv4.OptionRenewalTime, uint32(renewalTime.Seconds()))
		reply.Options.SetUint32(dhcpv4.OptionRebindingTime, uint32(rebindTime.Seconds()))
	}
}

// UpdateConfig updates the handler's configuration (for hot-reload).
func (h *Handler) UpdateConfig(cfg *config.Config) {
	h.cfg = cfg
	h.serverIP = cfg.ServerIP()
}

// UpdatePools updates the handler's pool map (for hot-reload).
func (h *Handler) UpdatePools(pools map[string][]*pool.Pool) {
	h.pools = pools
}
