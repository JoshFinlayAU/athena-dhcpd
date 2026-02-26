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

	fsm := &FSM{
		state:           initialState,
		role:            role,
		failoverTimeout: failoverTimeout,
		bus:             bus,
		logger:          logger,
	}

	logger.Info("HA FSM initialized",
		"role", role,
		"initial_state", string(initialState),
		"failover_timeout", failoverTimeout.String())

	return fsm
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

	isNowActive := newState == dhcpv4.HAStateActive || newState == dhcpv4.HAStatePartnerDown
	wasActive := oldState == dhcpv4.HAStateActive || oldState == dhcpv4.HAStatePartnerDown

	f.logger.Warn("HA state transition",
		"old_state", string(oldState),
		"new_state", string(newState),
		"role", f.role,
		"serving_dhcp", isNowActive,
		"reason", reason)

	if isNowActive && !wasActive {
		f.logger.Warn("this node is now ACTIVE and serving DHCP", "role", f.role)
	} else if !isNowActive && wasActive {
		f.logger.Warn("this node is no longer serving DHCP", "role", f.role, "new_state", string(newState))
	}

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
	prevHB := f.lastHeartbeat
	f.lastHeartbeat = time.Now()
	currentState := f.state
	f.mu.Unlock()

	switch currentState {
	case dhcpv4.HAStatePartnerDown:
		f.logger.Warn("peer came back after being down",
			"downtime", time.Since(prevHB).Round(time.Millisecond).String(),
			"current_state", string(currentState))
		f.transition(dhcpv4.HAStateRecovery, "peer reconnected")
	case dhcpv4.HAStateRecovery:
		// Stay in recovery until bulk sync completes
	case dhcpv4.HAStateActive, dhcpv4.HAStateStandby:
		if currentState == dhcpv4.HAStateActive || currentState == dhcpv4.HAStateStandby {
			f.transition(dhcpv4.HAStatePartnerUp, "peer heartbeat received")
		}
	}
}

// PeerDown is called when the peer is detected as unreachable (heartbeat timeout).
func (f *FSM) PeerDown() {
	f.mu.RLock()
	currentState := f.state
	lastHB := f.lastHeartbeat
	f.mu.RUnlock()

	f.logger.Warn("peer declared DOWN",
		"current_state", string(currentState),
		"role", f.role,
		"last_heartbeat_ago", time.Since(lastHB).Round(time.Millisecond).String(),
		"failover_timeout", f.failoverTimeout.String())

	switch currentState {
	case dhcpv4.HAStatePartnerUp, dhcpv4.HAStateStandby:
		f.transition(dhcpv4.HAStatePartnerDown, "peer heartbeat timeout")
		if f.role == "primary" {
			f.logger.Warn("primary node taking over as ACTIVE")
			f.transition(dhcpv4.HAStateActive, "primary claiming active after partner down")
		} else {
			f.logger.Warn("secondary node detected primary down — waiting for manual failover or primary recovery",
				"hint", "use POST /api/v2/ha/failover to force this node active")
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

	f.logger.Info("bulk sync complete", "current_state", string(currentState), "role", f.role)

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
	f.logger.Warn("manual failover requested",
		"role", f.role,
		"current_state", string(f.State()),
		"reason", reason)
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

	silence := time.Since(lastHB)
	if silence > f.failoverTimeout && currentState != dhcpv4.HAStatePartnerDown {
		f.logger.Warn("heartbeat timeout exceeded",
			"silence", silence.Round(time.Millisecond).String(),
			"timeout", f.failoverTimeout.String(),
			"current_state", string(currentState))
		f.PeerDown()
	}
}

// String returns a human-readable state description.
func (f *FSM) String() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return fmt.Sprintf("role=%s state=%s last_hb=%s", f.role, f.state, f.lastHeartbeat.Format(time.RFC3339))
}
