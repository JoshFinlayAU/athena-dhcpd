package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Server is the core DHCPv4 UDP server.
type Server struct {
	conn    *net.UDPConn
	handler *Handler
	logger  *slog.Logger
	addr    string
	iface   string
	wg      sync.WaitGroup
	done    chan struct{}
}

// NewServer creates a new DHCP server.
func NewServer(handler *Handler, iface, addr string, logger *slog.Logger) *Server {
	if addr == "" {
		addr = fmt.Sprintf(":%d", dhcpv4.ServerPort)
	}
	return &Server{
		handler: handler,
		logger:  logger,
		addr:    addr,
		iface:   iface,
		done:    make(chan struct{}),
	}
}

// Start begins listening for DHCP packets on UDP port 67.
func (s *Server) Start(ctx context.Context) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", s.addr)
	if err != nil {
		return fmt.Errorf("resolving UDP address %s: %w", s.addr, err)
	}

	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	s.conn = conn

	s.logger.Info("DHCP server started",
		"address", s.addr,
		"interface", s.iface)

	s.wg.Add(1)
	go s.serve(ctx)

	return nil
}

// serve is the main packet processing loop.
func (s *Server) serve(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		buf := GetBuffer()
		n, src, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				PutBuffer(buf)
				return
			default:
			}
			s.logger.Error("reading UDP packet", "error", err)
			PutBuffer(buf)
			continue
		}

		// Process packet in a goroutine to not block the listener
		s.wg.Add(1)
		go func(data []byte, length int, addr *net.UDPAddr) {
			defer s.wg.Done()
			defer PutBuffer(data)

			s.processPacket(ctx, data[:length], addr)
		}(buf, n, src)
	}
}

// processPacket handles a single DHCP packet.
func (s *Server) processPacket(ctx context.Context, data []byte, src *net.UDPAddr) {
	// Decode the packet
	pkt, err := DecodePacket(data)
	if err != nil {
		metrics.PacketErrors.WithLabelValues("decode").Inc()
		s.logger.Warn("dropping malformed packet",
			"error", err,
			"src", src.String(),
			"size", len(data))
		return
	}

	// Tag packet with receiving interface for multi-interface subnet matching
	pkt.ReceivingInterface = s.iface

	// Validate it's a BOOTREQUEST
	if pkt.Op != dhcpv4.OpCodeBootRequest {
		return
	}

	msgType := pkt.MessageType().String()
	metrics.PacketsReceived.WithLabelValues(msgType).Inc()
	start := time.Now()

	// Handle the packet
	reply, err := s.handler.HandlePacket(ctx, pkt, src)

	metrics.PacketProcessingDuration.WithLabelValues(msgType).Observe(time.Since(start).Seconds())

	if err != nil {
		metrics.PacketErrors.WithLabelValues("handler").Inc()
		s.logger.Error("handling DHCP packet",
			"error", err,
			"mac", pkt.CHAddr.String(),
			"msg_type", pkt.MessageType().String())
		return
	}

	if reply == nil {
		return // No response needed
	}

	// Encode and send the reply
	replyBytes, err := reply.Encode()
	if err != nil {
		metrics.PacketErrors.WithLabelValues("encode").Inc()
		s.logger.Error("encoding reply",
			"error", err,
			"mac", pkt.CHAddr.String())
		return
	}

	// Determine destination address
	dst := s.getReplyDestination(pkt, src)

	if _, err := s.conn.WriteToUDP(replyBytes, dst); err != nil {
		metrics.PacketErrors.WithLabelValues("send").Inc()
		s.logger.Error("sending reply",
			"error", err,
			"dst", dst.String(),
			"mac", pkt.CHAddr.String())
	} else {
		metrics.PacketsSent.WithLabelValues(reply.MessageType().String()).Inc()
	}
}

// getReplyDestination determines where to send the reply.
// RFC 2131 §4.1 — constructing the reply destination.
func (s *Server) getReplyDestination(request *Packet, src *net.UDPAddr) *net.UDPAddr {
	// If relayed, send back to relay agent (giaddr:67)
	if request.IsRelayed() {
		return &net.UDPAddr{
			IP:   request.GIAddr,
			Port: dhcpv4.ServerPort,
		}
	}

	// If broadcast flag is set, broadcast the reply
	if request.IsBroadcast() {
		return &net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: dhcpv4.ClientPort,
		}
	}

	// If client has an IP (renewal), unicast to it
	if !request.CIAddr.Equal(net.IPv4zero) {
		return &net.UDPAddr{
			IP:   request.CIAddr,
			Port: dhcpv4.ClientPort,
		}
	}

	// Default: broadcast
	return &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: dhcpv4.ClientPort,
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	close(s.done)
	if s.conn != nil {
		s.conn.Close()
	}
	s.wg.Wait()
	s.logger.Info("DHCP server stopped")
}

// Handler returns the packet handler.
func (s *Server) Handler() *Handler {
	return s.handler
}
