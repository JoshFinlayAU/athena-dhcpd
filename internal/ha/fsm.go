package ha

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// FSM implements the failover state machine with explicit states:
// PARTNER_UP, PARTNER_DOWN, ACTIVE, STANDBY, RECOVERY.
type FSM struct {
	state           dhcpv4.HAState
	role            string // "primary" or "secondary"
	lastHeartbeat   time.Time
	failoverTimeout time.Duration
	bus             *events.Bus
	logger          *slog.Logger
	mu              sync.RWMutex
	onStateChange   func(old, new dhcpv4.HAState)
}

// NewFSM creates a new failover state machine.
func NewFSM(role string, failoverTimeout time.Duration, bus *events.Bus, logger *slog.Logger) *FSM {
	initialState := dhcpv4.HAStateStandby
	if role == "primary" {
		initialState = dhcpv4.HAStateActive
	}

	return &FSM{
		state:           initialState,
		role:            role,
		failoverTimeout: failoverTimeout,
		bus:             bus,
		logger:          logger,
	}
}

// State returns the current HA state.
func (f *FSM) State() dhcpv4.HAState {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state
}

// Role returns the configured role.
func (f *FSM) Role() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.role
}

// IsActive returns true if this node should be serving DHCP requests.
func (f *FSM) IsActive() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state == dhcpv4.HAStateActive || f.state == dhcpv4.HAStatePartnerDown
}

// LastHeartbeat returns when the last heartbeat was received from the peer.
func (f *FSM) LastHeartbeat() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastHeartbeat
}

// OnStateChange sets a callback for state transitions.
func (f *FSM) OnStateChange(fn func(old, new dhcpv4.HAState)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onStateChange = fn
}

// transition changes state with logging and event emission.
func (f *FSM) transition(newState dhcpv4.HAState, reason string) {
	f.mu.Lock()
	oldState := f.state
	if oldState == newState {
		f.mu.Unlock()
		return
	}
	f.state = newState
	cb := f.onStateChange
	f.mu.Unlock()

	f.logger.Info("HA state transition",
		"old_state", string(oldState),
		"new_state", string(newState),
		"reason", reason)

	if cb != nil {
		cb(oldState, newState)
	}

	f.bus.Publish(events.Event{
		Type:      events.EventHAFailover,
		Timestamp: time.Now(),
		HA: &events.HAData{
			OldRole: string(oldState),
			NewRole: string(newState),
		},
		Reason: reason,
	})
}

// PeerUp is called when the peer connection is established or a heartbeat is received.
func (f *FSM) PeerUp() {
	f.mu.Lock()
	f.lastHeartbeat = time.Now()
	currentState := f.state
	f.mu.Unlock()

	switch currentState {
	case dhcpv4.HAStatePartnerDown:
		// Partner is back — transition to recovery
		f.transition(dhcpv4.HAStateRecovery, "peer reconnected")
	case dhcpv4.HAStateRecovery:
		// Stay in recovery until bulk sync completes
	case dhcpv4.HAStateActive, dhcpv4.HAStateStandby:
		// Update to PARTNER_UP if not already
		if currentState == dhcpv4.HAStateActive || currentState == dhcpv4.HAStateStandby {
			f.transition(dhcpv4.HAStatePartnerUp, "peer heartbeat received")
		}
	}
}

// PeerDown is called when the peer is detected as unreachable (heartbeat timeout).
func (f *FSM) PeerDown() {
	f.mu.RLock()
	currentState := f.state
	f.mu.RUnlock()

	switch currentState {
	case dhcpv4.HAStatePartnerUp, dhcpv4.HAStateStandby:
		f.transition(dhcpv4.HAStatePartnerDown, "peer heartbeat timeout")
		// If we're the primary (or only node), claim active
		if f.role == "primary" {
			f.transition(dhcpv4.HAStateActive, "primary claiming active after partner down")
		}
	case dhcpv4.HAStateRecovery:
		f.transition(dhcpv4.HAStatePartnerDown, "peer lost during recovery")
	}
}

// BulkSyncComplete is called when bulk synchronisation finishes after recovery.
func (f *FSM) BulkSyncComplete() {
	f.mu.RLock()
	currentState := f.state
	f.mu.RUnlock()

	if currentState == dhcpv4.HAStateRecovery {
		if f.role == "primary" {
			f.transition(dhcpv4.HAStateActive, "bulk sync complete — primary resuming active")
		} else {
			f.transition(dhcpv4.HAStateStandby, "bulk sync complete — secondary resuming standby")
		}
	}
}

// ClaimActive forces this node to the ACTIVE state (manual failover or admin action).
func (f *FSM) ClaimActive(reason string) {
	f.transition(dhcpv4.HAStateActive, fmt.Sprintf("manual claim: %s", reason))
}

// CheckHeartbeatTimeout checks if the peer heartbeat has timed out.
// Should be called periodically (e.g., every second).
func (f *FSM) CheckHeartbeatTimeout() {
	f.mu.RLock()
	lastHB := f.lastHeartbeat
	currentState := f.state
	f.mu.RUnlock()

	if lastHB.IsZero() {
		return // No heartbeat received yet
	}

	if time.Since(lastHB) > f.failoverTimeout && currentState != dhcpv4.HAStatePartnerDown {
		f.PeerDown()
	}
}

// String returns a human-readable state description.
func (f *FSM) String() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return fmt.Sprintf("role=%s state=%s last_hb=%s", f.role, f.state, f.lastHeartbeat.Format(time.RFC3339))
}
