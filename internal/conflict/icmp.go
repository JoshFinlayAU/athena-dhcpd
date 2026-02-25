package conflict

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// ICMPProber sends ICMP Echo Requests to detect IP conflicts on remote/relayed subnets.
// Fallback method when ARP can't cross L3 boundaries (RFC 792).
// The ICMP socket is opened once at startup and shared across all probes.
type ICMPProber struct {
	conn      *icmp.PacketConn
	logger    *slog.Logger
	available bool
	seq       uint16
	mu        sync.Mutex
}

// NewICMPProber creates a new ICMP prober.
// If raw ICMP socket creation fails (missing CAP_NET_RAW), logs a LOUD warning
// and returns a prober that always reports "clear".
func NewICMPProber(logger *slog.Logger) (*ICMPProber, error) {
	p := &ICMPProber{
		logger: logger,
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		logger.Error("FAILED TO OPEN ICMP SOCKET — IP conflict detection via ICMP is DISABLED",
			"error", err,
			"hint", "Grant CAP_NET_RAW capability or run as root")
		p.available = false
		return p, nil
	}

	p.conn = conn
	p.available = true
	logger.Info("ICMP prober initialized")

	return p, nil
}

// Available returns true if the ICMP prober has a working socket.
func (p *ICMPProber) Available() bool {
	return p.available
}

// Close closes the ICMP socket.
func (p *ICMPProber) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Probe sends an ICMP Echo Request to the target IP and waits for a reply.
// Returns true if a reply is received (conflict detected), false on timeout.
func (p *ICMPProber) Probe(ctx context.Context, targetIP net.IP) (bool, error) {
	if !p.available {
		return false, nil // Degraded mode — assume clear
	}

	p.mu.Lock()
	p.seq++
	seq := p.seq
	p.mu.Unlock()

	start := time.Now()

	// Build ICMP Echo Request
	msg := &icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  int(seq),
			Data: []byte("athena-probe"),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return false, fmt.Errorf("marshalling ICMP echo request: %w", err)
	}

	dst := &net.IPAddr{IP: targetIP}

	// Set deadline from context
	deadline, ok := ctx.Deadline()
	if ok {
		if err := p.conn.SetDeadline(deadline); err != nil {
			return false, fmt.Errorf("setting ICMP deadline: %w", err)
		}
	}

	// Send ICMP Echo Request
	if _, err := p.conn.WriteTo(msgBytes, dst); err != nil {
		return false, fmt.Errorf("sending ICMP echo to %s: %w", targetIP, err)
	}

	// Wait for reply
	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			p.logger.Debug("ICMP probe timeout (clear)",
				"target_ip", targetIP.String(),
				"duration", time.Since(start).String())
			return false, nil
		default:
		}

		n, peer, err := p.conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				p.logger.Debug("ICMP probe timeout (clear)",
					"target_ip", targetIP.String(),
					"duration", time.Since(start).String())
				return false, nil
			}
			return false, fmt.Errorf("reading ICMP reply: %w", err)
		}

		// Parse the reply
		reply, err := icmp.ParseMessage(1, buf[:n]) // 1 = ICMPv4
		if err != nil {
			continue
		}

		if reply.Type != ipv4.ICMPTypeEchoReply {
			continue
		}

		// Check if this reply is from our target
		if echo, ok := reply.Body.(*icmp.Echo); ok {
			if echo.ID == os.Getpid()&0xffff && echo.Seq == int(seq) {
				p.logger.Debug("ICMP probe reply received (conflict)",
					"target_ip", targetIP.String(),
					"responder", peer.String(),
					"duration", time.Since(start).String())
				return true, nil
			}
		}
	}
}
