package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Peer manages the TCP connection to the HA partner node.
type Peer struct {
	cfg               *config.HAConfig
	fsm               *FSM
	leaseStore        *lease.Store
	bus               *events.Bus
	logger            *slog.Logger
	conn              net.Conn
	listener          net.Listener
	heartbeatInterval time.Duration
	mu                sync.Mutex
	done              chan struct{}
	wg                sync.WaitGroup
	onLeaseUpdate     func(LeaseUpdatePayload)
	onConflictUpdate  func(ConflictUpdatePayload)
	onConfigSync      func(ConfigSyncPayload)
	onAdjacencyFormed func()
	lastConnErr       string
	lastConnErrAt     time.Time
}

// NewPeer creates a new HA peer manager.
func NewPeer(cfg *config.HAConfig, fsm *FSM, store *lease.Store, bus *events.Bus, logger *slog.Logger) (*Peer, error) {
	hbInterval, err := time.ParseDuration(cfg.HeartbeatInterval)
	if err != nil {
		hbInterval = time.Second
	}

	return &Peer{
		cfg:               cfg,
		fsm:               fsm,
		leaseStore:        store,
		bus:               bus,
		logger:            logger,
		heartbeatInterval: hbInterval,
		done:              make(chan struct{}),
	}, nil
}

// OnLeaseUpdate sets a callback for incoming lease updates from the peer.
func (p *Peer) OnLeaseUpdate(fn func(LeaseUpdatePayload)) {
	p.onLeaseUpdate = fn
}

// OnConflictUpdate sets a callback for incoming conflict updates from the peer.
func (p *Peer) OnConflictUpdate(fn func(ConflictUpdatePayload)) {
	p.onConflictUpdate = fn
}

// OnConfigSync sets a callback for incoming config sync messages from the peer.
func (p *Peer) OnConfigSync(fn func(ConfigSyncPayload)) {
	p.onConfigSync = fn
}

// OnAdjacencyFormed sets a callback that fires every time a peer connection
// is established (both inbound and outbound). The primary uses this to push
// its full config to the backup.
func (p *Peer) OnAdjacencyFormed(fn func()) {
	p.onAdjacencyFormed = fn
}

// SendConfigSync sends a config section update to the peer.
func (p *Peer) SendConfigSync(section string, data []byte) error {
	msg, err := NewConfigSync(section, data)
	if err != nil {
		return fmt.Errorf("creating config sync message for %s: %w", section, err)
	}
	return p.sendMessage(msg)
}

// SendFullConfigSync sends all config sections to the peer.
func (p *Peer) SendFullConfigSync(sections map[string][]byte) error {
	var firstErr error
	for section, data := range sections {
		if err := p.SendConfigSync(section, data); err != nil {
			p.logger.Error("failed to send config section to peer",
				"section", section, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Start begins the HA peer connection (listen + connect).
func (p *Peer) Start(ctx context.Context) error {
	// Start listener for incoming peer connections
	listener, err := net.Listen("tcp", p.cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", p.cfg.ListenAddress, err)
	}
	p.listener = listener

	p.logger.Info("HA peer listener started",
		"listen", p.cfg.ListenAddress,
		"peer", p.cfg.PeerAddress,
		"role", p.cfg.Role,
		"heartbeat_interval", p.heartbeatInterval.String())

	// Accept incoming connections
	p.wg.Add(1)
	go p.acceptLoop(ctx)

	// Only secondary connects outbound to primary.
	// Primary is active and just listens for the secondary to connect.
	if p.cfg.Role == "secondary" {
		p.wg.Add(1)
		go p.connectLoop(ctx)
	} else {
		p.logger.Info("primary role — waiting for secondary to connect inbound")
	}

	// Heartbeat sender
	p.wg.Add(1)
	go p.heartbeatLoop(ctx)

	// Heartbeat timeout checker
	p.wg.Add(1)
	go p.timeoutLoop(ctx)

	return nil
}

// Stop shuts down the HA peer connection.
func (p *Peer) Stop() {
	close(p.done)
	if p.listener != nil {
		p.listener.Close()
	}
	p.mu.Lock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
	p.logger.Info("HA peer stopped")
}

// SendLeaseUpdate sends a lease change to the peer.
func (p *Peer) SendLeaseUpdate(l *lease.Lease) error {
	msg, err := NewLeaseUpdate(
		l.IP, l.MAC, l.ClientID, l.Hostname,
		l.Subnet, l.Pool, string(l.State),
		l.Start, l.Expiry, l.UpdateSeq,
	)
	if err != nil {
		return fmt.Errorf("creating lease update message: %w", err)
	}
	return p.sendMessage(msg)
}

// SendConflictUpdate sends a conflict table entry to the peer.
func (p *Peer) SendConflictUpdate(ip net.IP, detectedAt time.Time, method, responderMAC, subnet string,
	probeCount int, permanent bool) error {
	msg, err := NewConflictUpdate(ip, detectedAt, method, responderMAC, subnet, probeCount, permanent)
	if err != nil {
		return fmt.Errorf("creating conflict update message: %w", err)
	}
	return p.sendMessage(msg)
}

// sendMessage sends a message to the connected peer.
func (p *Peer) sendMessage(msg *Message) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("no peer connection")
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		return err
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("writing to peer: %w", err)
	}

	return nil
}

// acceptLoop listens for incoming peer connections.
func (p *Peer) acceptLoop(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		default:
		}

		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
			}
			p.logger.Error("accepting peer connection", "error", err)
			continue
		}

		p.logger.Info("inbound peer connection accepted",
			"remote", conn.RemoteAddr().String(),
			"local", conn.LocalAddr().String())
		p.setConn(conn)
		p.fsm.PeerUp()

		// Notify adjacency formed (primary pushes config here)
		if p.onAdjacencyFormed != nil {
			p.logger.Info("HA adjacency formed (inbound)", "remote", conn.RemoteAddr().String())
			go p.onAdjacencyFormed()
		}

		// Handle incoming messages
		p.wg.Add(1)
		go func(c net.Conn) {
			defer p.wg.Done()
			p.handleConnection(ctx, c)
		}(conn)
	}
}

// connectLoop attempts to connect to the peer (outbound).
func (p *Peer) connectLoop(ctx context.Context) {
	defer p.wg.Done()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		default:
		}

		p.mu.Lock()
		hasConn := p.conn != nil
		p.mu.Unlock()

		if hasConn {
			time.Sleep(5 * time.Second)
			continue
		}

		conn, err := net.DialTimeout("tcp", p.cfg.PeerAddress, 5*time.Second)
		if err != nil {
			p.mu.Lock()
			p.lastConnErr = err.Error()
			p.lastConnErrAt = time.Now()
			p.mu.Unlock()
			if backoff > time.Second {
				p.logger.Warn("failed to connect to peer, retrying",
					"address", p.cfg.PeerAddress,
					"next_retry", backoff.String(),
					"error", err)
			} else {
				p.logger.Debug("failed to connect to peer",
					"address", p.cfg.PeerAddress, "error", err)
			}
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		p.mu.Lock()
		p.lastConnErr = ""
		p.mu.Unlock()
		backoff = time.Second
		p.logger.Info("outbound peer connection established", "address", p.cfg.PeerAddress)
		p.setConn(conn)
		p.fsm.PeerUp()

		// Notify adjacency formed (primary pushes config here)
		if p.onAdjacencyFormed != nil {
			p.logger.Info("HA adjacency formed (outbound)", "address", p.cfg.PeerAddress)
			go p.onAdjacencyFormed()
		}

		// Handle incoming messages on this connection
		p.wg.Add(1)
		go func(c net.Conn) {
			defer p.wg.Done()
			p.handleConnection(ctx, c)
		}(conn)
	}
}

// handleConnection processes messages from a peer connection.
func (p *Peer) handleConnection(ctx context.Context, conn net.Conn) {
	remote := conn.RemoteAddr().String()
	defer func() {
		conn.Close()
		p.mu.Lock()
		wasActive := p.conn == conn
		if wasActive {
			p.conn = nil
		}
		p.mu.Unlock()
		if wasActive {
			p.logger.Warn("peer connection lost", "remote", remote)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(p.heartbeatInterval * 3))
		msg, err := DecodeMessage(conn)
		if err != nil {
			p.logger.Warn("peer connection read error", "remote", remote, "error", err)
			return
		}

		p.handleMessage(msg)
	}
}

// handleMessage processes a single received HA message.
func (p *Peer) handleMessage(msg *Message) {
	switch msg.Type {
	case dhcpv4.HAMsgHeartbeat:
		var hb HeartbeatPayload
		if err := json.Unmarshal(msg.Payload, &hb); err != nil {
			p.logger.Error("failed to unmarshal heartbeat", "error", err)
			return
		}
		metrics.HAHeartbeatsReceived.Inc()
		p.fsm.PeerUp()
		p.logger.Debug("heartbeat received",
			"peer_state", hb.State,
			"peer_leases", hb.LeaseCount,
			"peer_seq", hb.Seq)

	case dhcpv4.HAMsgLeaseUpdate:
		var lu LeaseUpdatePayload
		if err := json.Unmarshal(msg.Payload, &lu); err != nil {
			metrics.HASyncErrors.Inc()
			p.logger.Error("failed to unmarshal lease update", "error", err)
			return
		}
		metrics.HASyncOperations.WithLabelValues("lease_update").Inc()
		if p.onLeaseUpdate != nil {
			p.onLeaseUpdate(lu)
		}

	case dhcpv4.HAMsgConflictUpdate:
		var cu ConflictUpdatePayload
		if err := json.Unmarshal(msg.Payload, &cu); err != nil {
			metrics.HASyncErrors.Inc()
			p.logger.Error("failed to unmarshal conflict update", "error", err)
			return
		}
		metrics.HASyncOperations.WithLabelValues("conflict_update").Inc()
		if p.onConflictUpdate != nil {
			p.onConflictUpdate(cu)
		}

	case dhcpv4.HAMsgBulkStart:
		p.logger.Info("peer bulk sync starting")

	case dhcpv4.HAMsgBulkEnd:
		p.logger.Info("peer bulk sync complete")
		p.fsm.BulkSyncComplete()

	case dhcpv4.HAMsgConfigSync:
		var cs ConfigSyncPayload
		if err := json.Unmarshal(msg.Payload, &cs); err != nil {
			metrics.HASyncErrors.Inc()
			p.logger.Error("failed to unmarshal config sync", "error", err)
			return
		}
		metrics.HASyncOperations.WithLabelValues("config_sync").Inc()
		p.logger.Info("config sync received from peer", "section", cs.Section)
		if p.onConfigSync != nil {
			p.onConfigSync(cs)
		}

	case dhcpv4.HAMsgFailoverClaim:
		var fc FailoverClaimPayload
		if err := json.Unmarshal(msg.Payload, &fc); err != nil {
			p.logger.Error("failed to unmarshal failover claim", "error", err)
			return
		}
		p.logger.Warn("peer claimed active role", "reason", fc.Reason)
		// If peer claims active, we become standby
		p.fsm.transition(dhcpv4.HAStateStandby, "peer claimed active: "+fc.Reason)

	default:
		p.logger.Warn("unknown HA message type", "type", msg.Type)
	}
}

// heartbeatLoop sends periodic heartbeats to the peer.
func (p *Peer) heartbeatLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.heartbeatInterval)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case <-ticker.C:
			msg, err := NewHeartbeat(
				string(p.fsm.State()),
				p.leaseStore.Count(),
				p.leaseStore.NextSeq(),
				time.Since(startTime),
			)
			if err != nil {
				continue
			}
			if err := p.sendMessage(msg); err != nil {
				p.logger.Debug("heartbeat send skipped", "error", err)
			} else {
				metrics.HAHeartbeatsSent.Inc()
			}
		}
	}
}

// timeoutLoop periodically checks for heartbeat timeout.
func (p *Peer) timeoutLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case <-ticker.C:
			p.fsm.CheckHeartbeatTimeout()
		}
	}
}

// setConn sets the active peer connection.
func (p *Peer) setConn(conn net.Conn) {
	p.mu.Lock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = conn
	p.mu.Unlock()
}

// Connected returns true if the peer connection is active.
func (p *Peer) Connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn != nil
}

// LastConnError returns the last outbound connection error and when it occurred.
func (p *Peer) LastConnError() (string, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastConnErr, p.lastConnErrAt
}

// FSM returns the failover state machine.
func (p *Peer) FSM() *FSM {
	return p.fsm
}
