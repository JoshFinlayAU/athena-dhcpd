package rogue

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// ProbeConfig holds settings for active rogue DHCP server probing.
type ProbeConfig struct {
	Interface string
	Interval  time.Duration // how often to send a probe (default 5m)
	Timeout   time.Duration // how long to wait for replies per probe (default 3s)
}

// StartProbing begins periodic active DHCPDISCOVER probes on the network.
// Any DHCPOFFER from a server not in ownIPs is flagged as rogue.
func (d *Detector) StartProbing(ctx context.Context, cfg ProbeConfig) {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}

	go d.probeLoop(ctx, cfg)
}

// ScanNow triggers a single immediate probe and returns results.
func (d *Detector) ScanNow(ctx context.Context, cfg ProbeConfig) ([]ServerEntry, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}
	found, err := d.runProbe(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (d *Detector) probeLoop(ctx context.Context, cfg ProbeConfig) {
	// Initial probe shortly after startup
	time.Sleep(10 * time.Second)

	d.logger.Info("rogue DHCP probe loop started",
		"interface", cfg.Interface,
		"interval", cfg.Interval.String(),
		"timeout", cfg.Timeout.String())

	if _, err := d.runProbe(ctx, cfg); err != nil {
		d.logger.Warn("rogue probe failed", "error", err)
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.done:
			return
		case <-ticker.C:
			if _, err := d.runProbe(ctx, cfg); err != nil {
				d.logger.Warn("rogue probe failed", "error", err)
			}
		}
	}
}

// runProbe sends a DHCPDISCOVER broadcast and collects DHCPOFFER replies.
func (d *Detector) runProbe(ctx context.Context, cfg ProbeConfig) ([]ServerEntry, error) {
	// Generate a random XID and fake MAC for this probe
	xid := make([]byte, 4)
	if _, err := rand.Read(xid); err != nil {
		return nil, fmt.Errorf("generating random XID: %w", err)
	}
	fakeMAC := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x00} // locally-administered
	if _, err := rand.Read(fakeMAC[2:]); err != nil {
		return nil, fmt.Errorf("generating random MAC: %w", err)
	}

	// Build a minimal DHCPDISCOVER packet
	pkt := buildDiscover(xid, fakeMAC)

	// Open a UDP socket on port 68 (DHCP client port) to send and receive
	listenAddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ClientPort}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		// Port 68 may be in use — try an ephemeral port as fallback
		// (won't receive broadcast replies, but direct replies work)
		listenAddr = &net.UDPAddr{IP: net.IPv4zero, Port: 0}
		conn, err = net.ListenUDP("udp4", listenAddr)
		if err != nil {
			return nil, fmt.Errorf("opening probe socket: %w", err)
		}
		d.logger.Debug("rogue probe using ephemeral port (port 68 in use)")
	}
	defer conn.Close()

	// Send DHCPDISCOVER to broadcast:67
	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ServerPort}
	if _, err := conn.WriteToUDP(pkt, dst); err != nil {
		return nil, fmt.Errorf("sending DHCPDISCOVER probe: %w", err)
	}

	d.logger.Debug("rogue probe DHCPDISCOVER sent",
		"xid", fmt.Sprintf("%08x", xid),
		"fake_mac", fakeMAC.String())

	// Listen for DHCPOFFER replies until timeout
	probeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var found []ServerEntry
	buf := make([]byte, 1500)

	for {
		deadline, _ := probeCtx.Deadline()
		conn.SetReadDeadline(deadline)

		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Timeout or context cancelled — done listening
			break
		}

		serverIP, offeredIP, msgType := parseOffer(buf[:n], xid)
		if serverIP == nil {
			continue // Not a DHCPOFFER or XID mismatch
		}

		sip := serverIP.String()
		d.mu.RLock()
		isOwn := d.ownIPs[sip]
		d.mu.RUnlock()

		if isOwn {
			d.logger.Debug("rogue probe got OFFER from ourselves", "server_ip", sip)
			continue
		}

		// Resolve server MAC from OS ARP cache (populated by the UDP exchange)
		srcMAC := lookupARPCache(serverIP)
		d.logger.Warn("rogue probe detected DHCP server",
			"server_ip", sip,
			"server_mac", srcMAC,
			"offered_ip", offeredIP,
			"msg_type", msgType,
			"src", src.String())

		d.ReportOffer(serverIP, offeredIP, fakeMAC, srcMAC, cfg.Interface)

		found = append(found, ServerEntry{
			ServerIP:  sip,
			LastSeen:  time.Now(),
			Interface: cfg.Interface,
		})
	}

	d.logger.Info("rogue probe complete",
		"servers_found", len(found),
		"own_excluded", true)

	return found, nil
}

// buildDiscover creates a minimal DHCPDISCOVER packet.
func buildDiscover(xid []byte, mac net.HardwareAddr) []byte {
	// DHCP packet: 236 bytes header + 4 magic cookie + options
	pkt := make([]byte, 300)

	pkt[0] = byte(dhcpv4.OpCodeBootRequest)    // op
	pkt[1] = byte(dhcpv4.HardwareTypeEthernet) // htype
	pkt[2] = 6                                 // hlen (MAC = 6 bytes)
	pkt[3] = 0                                 // hops

	// XID (bytes 4-7)
	copy(pkt[4:8], xid)

	// secs (bytes 8-9): 0
	// flags (bytes 10-11): broadcast flag
	binary.BigEndian.PutUint16(pkt[10:12], 0x8000)

	// ciaddr, yiaddr, siaddr, giaddr: all zero (bytes 12-27)

	// chaddr (bytes 28-43): client hardware address
	copy(pkt[28:34], mac)

	// sname (bytes 44-107): zero
	// file (bytes 108-235): zero

	// Magic cookie (bytes 236-239)
	copy(pkt[236:240], dhcpv4.MagicCookie)

	// Options start at byte 240
	i := 240

	// Option 53: DHCP Message Type = DISCOVER
	pkt[i] = byte(dhcpv4.OptionDHCPMessageType)
	pkt[i+1] = 1
	pkt[i+2] = byte(dhcpv4.MessageTypeDiscover)
	i += 3

	// Option 55: Parameter Request List (minimal)
	pkt[i] = byte(dhcpv4.OptionParameterRequestList)
	pkt[i+1] = 3
	pkt[i+2] = byte(dhcpv4.OptionSubnetMask)
	pkt[i+3] = byte(dhcpv4.OptionRouter)
	pkt[i+4] = byte(dhcpv4.OptionDomainNameServer)
	i += 5

	// End option
	pkt[i] = byte(dhcpv4.OptionEnd)

	return pkt
}

// parseOffer extracts server IP and offered IP from a DHCPOFFER reply.
// Returns nil if the packet isn't a valid OFFER matching our XID.
func parseOffer(data []byte, expectedXID []byte) (serverIP, offeredIP net.IP, msgType string) {
	if len(data) < 240 {
		return nil, nil, ""
	}

	// Check op = BOOTREPLY
	if data[0] != byte(dhcpv4.OpCodeBootReply) {
		return nil, nil, ""
	}

	// Check XID matches
	if data[4] != expectedXID[0] || data[5] != expectedXID[1] ||
		data[6] != expectedXID[2] || data[7] != expectedXID[3] {
		return nil, nil, ""
	}

	// Check magic cookie
	if data[236] != 99 || data[237] != 130 || data[238] != 83 || data[239] != 99 {
		return nil, nil, ""
	}

	// Parse options to find message type and server identifier
	var isOffer bool
	var srvIP net.IP

	offeredIP = net.IP(data[16:20]).To4() // yiaddr
	siaddr := net.IP(data[20:24]).To4()   // siaddr (fallback server IP)

	i := 240
	for i < len(data) {
		opt := data[i]
		if opt == byte(dhcpv4.OptionEnd) {
			break
		}
		if opt == byte(dhcpv4.OptionPad) {
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		optLen := int(data[i+1])
		if i+2+optLen > len(data) {
			break
		}
		optData := data[i+2 : i+2+optLen]

		switch dhcpv4.OptionCode(opt) {
		case dhcpv4.OptionDHCPMessageType:
			if optLen == 1 {
				mt := dhcpv4.MessageType(optData[0])
				msgType = mt.String()
				isOffer = mt == dhcpv4.MessageTypeOffer || mt == dhcpv4.MessageTypeAck
			}
		case dhcpv4.OptionServerIdentifier:
			if optLen == 4 {
				srvIP = net.IP(optData).To4()
			}
		}

		i += 2 + optLen
	}

	if !isOffer {
		return nil, nil, ""
	}

	// Use server identifier option if present, otherwise fall back to siaddr
	if srvIP == nil {
		srvIP = siaddr
	}
	if srvIP.Equal(net.IPv4zero) {
		return nil, nil, ""
	}

	return srvIP, offeredIP, msgType
}

// lookupARPCache reads the OS ARP cache to find the MAC address for an IP.
// On Linux this reads /proc/net/arp. Returns nil if not found.
func lookupARPCache(ip net.IP) net.HardwareAddr {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return nil
	}
	defer f.Close()

	target := ip.String()
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		// fields: IP, HWtype, Flags, HWaddress, Mask, Device
		if fields[0] == target && fields[3] != "00:00:00:00:00:00" {
			mac, err := net.ParseMAC(fields[3])
			if err == nil {
				return mac
			}
		}
	}
	return nil
}

// Stop signals the probe loop to exit.
func (d *Detector) Stop() {
	select {
	case <-d.done:
	default:
		close(d.done)
	}
}
