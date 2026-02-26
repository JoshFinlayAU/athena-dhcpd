package ha

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func newTestFSM(role string) (*FSM, *events.Bus) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := events.NewBus(100, logger)
	go bus.Start()
	fsm := NewFSM(role, 5*time.Second, bus, logger)
	return fsm, bus
}

func TestFSMInitialState(t *testing.T) {
	primary, bus := newTestFSM("primary")
	defer bus.Stop()
	if primary.State() != dhcpv4.HAStateActive {
		t.Errorf("primary initial state = %s, want ACTIVE", primary.State())
	}

	secondary, bus2 := newTestFSM("secondary")
	defer bus2.Stop()
	if secondary.State() != dhcpv4.HAStateStandby {
		t.Errorf("secondary initial state = %s, want STANDBY", secondary.State())
	}
}

func TestFSMIsActive(t *testing.T) {
	primary, bus := newTestFSM("primary")
	defer bus.Stop()
	if !primary.IsActive() {
		t.Error("primary should be active")
	}

	secondary, bus2 := newTestFSM("secondary")
	defer bus2.Stop()
	if secondary.IsActive() {
		t.Error("secondary should not be active initially")
	}
}

func TestFSMPeerUp(t *testing.T) {
	// Primary stays ACTIVE when peer connects
	primary, bus := newTestFSM("primary")
	defer bus.Stop()

	primary.PeerUp()

	if primary.State() != dhcpv4.HAStateActive {
		t.Errorf("primary state after PeerUp = %s, want ACTIVE", primary.State())
	}
	if primary.LastHeartbeat().IsZero() {
		t.Error("LastHeartbeat should be set after PeerUp")
	}

	// Secondary transitions to PARTNER_UP when peer connects
	secondary, bus2 := newTestFSM("secondary")
	defer bus2.Stop()

	secondary.PeerUp()

	if secondary.State() != dhcpv4.HAStatePartnerUp {
		t.Errorf("secondary state after PeerUp = %s, want PARTNER_UP", secondary.State())
	}
}

func TestFSMPeerDown(t *testing.T) {
	fsm, bus := newTestFSM("primary")
	defer bus.Stop()

	// Primary is ACTIVE, peer heartbeat sets timestamp
	fsm.PeerUp()

	// Then peer goes down — primary goes ACTIVE → PARTNER_DOWN (still serving)
	fsm.PeerDown()

	if fsm.State() != dhcpv4.HAStatePartnerDown {
		t.Errorf("primary state after PeerDown = %s, want PARTNER_DOWN", fsm.State())
	}
	if !fsm.IsActive() {
		t.Error("primary should still be active in PARTNER_DOWN")
	}
}

func TestFSMSecondaryPeerDown(t *testing.T) {
	fsm, bus := newTestFSM("secondary")
	defer bus.Stop()

	fsm.PeerUp()
	fsm.PeerDown()

	// Secondary goes to PARTNER_DOWN but doesn't auto-claim active
	if fsm.State() != dhcpv4.HAStatePartnerDown {
		t.Errorf("secondary state after PeerDown = %s, want PARTNER_DOWN", fsm.State())
	}
}

func TestFSMRecovery(t *testing.T) {
	fsm, bus := newTestFSM("primary")
	defer bus.Stop()

	// Simulate: partner up → partner down → partner back
	fsm.PeerUp()
	fsm.PeerDown()

	// Now in ACTIVE (primary claimed it)
	// Simulate partner reconnecting while we're in PARTNER_DOWN...
	// First force to PARTNER_DOWN to test recovery
	fsm.transition(dhcpv4.HAStatePartnerDown, "test")
	fsm.PeerUp()

	if fsm.State() != dhcpv4.HAStateRecovery {
		t.Errorf("state after partner reconnect = %s, want RECOVERY", fsm.State())
	}
}

func TestFSMBulkSyncComplete(t *testing.T) {
	fsm, bus := newTestFSM("primary")
	defer bus.Stop()

	fsm.transition(dhcpv4.HAStateRecovery, "test")
	fsm.BulkSyncComplete()

	if fsm.State() != dhcpv4.HAStateActive {
		t.Errorf("primary state after bulk sync = %s, want ACTIVE", fsm.State())
	}

	// Test secondary
	fsm2, bus2 := newTestFSM("secondary")
	defer bus2.Stop()
	fsm2.transition(dhcpv4.HAStateRecovery, "test")
	fsm2.BulkSyncComplete()

	if fsm2.State() != dhcpv4.HAStateStandby {
		t.Errorf("secondary state after bulk sync = %s, want STANDBY", fsm2.State())
	}
}

func TestFSMClaimActive(t *testing.T) {
	fsm, bus := newTestFSM("secondary")
	defer bus.Stop()

	fsm.ClaimActive("manual failover")

	if fsm.State() != dhcpv4.HAStateActive {
		t.Errorf("state after ClaimActive = %s, want ACTIVE", fsm.State())
	}
}

func TestFSMHeartbeatTimeout(t *testing.T) {
	fsm, bus := newTestFSM("primary")
	defer bus.Stop()
	fsm.failoverTimeout = 50 * time.Millisecond

	fsm.PeerUp()
	time.Sleep(100 * time.Millisecond)
	fsm.CheckHeartbeatTimeout()

	// Primary was ACTIVE → should now be PARTNER_DOWN (still serving)
	if fsm.State() != dhcpv4.HAStatePartnerDown {
		t.Errorf("state after heartbeat timeout = %s, want PARTNER_DOWN", fsm.State())
	}
	if !fsm.IsActive() {
		t.Error("primary should still be active after heartbeat timeout")
	}
}

func TestFSMOnStateChange(t *testing.T) {
	// Use secondary — it transitions STANDBY → PARTNER_UP on PeerUp
	fsm, bus := newTestFSM("secondary")
	defer bus.Stop()

	var oldState, newState dhcpv4.HAState
	fsm.OnStateChange(func(o, n dhcpv4.HAState) {
		oldState = o
		newState = n
	})

	fsm.PeerUp()

	if oldState != dhcpv4.HAStateStandby {
		t.Errorf("oldState = %s, want STANDBY", oldState)
	}
	if newState != dhcpv4.HAStatePartnerUp {
		t.Errorf("newState = %s, want PARTNER_UP", newState)
	}
}

func TestFSMString(t *testing.T) {
	fsm, bus := newTestFSM("primary")
	defer bus.Stop()

	s := fsm.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}
