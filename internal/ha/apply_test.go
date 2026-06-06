package ha

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func newTestStore(t *testing.T) *lease.Store {
	t.Helper()
	s, err := lease.NewStore(filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleLease(ip string, updated time.Time, state dhcpv4.LeaseState, seq uint64) *lease.Lease {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	return &lease.Lease{
		IP:          net.ParseIP(ip),
		MAC:         mac,
		Subnet:      "192.168.1.0/24",
		State:       state,
		Start:       updated,
		Expiry:      updated.Add(time.Hour),
		LastUpdated: updated,
		UpdateSeq:   seq,
	}
}

// payloadFor builds the wire payload that a peer would send for the given lease,
// exercising the same encode path used by SendLeaseUpdate.
func payloadFor(t *testing.T, l *lease.Lease) LeaseUpdatePayload {
	t.Helper()
	msg, err := leaseUpdateMessage(l)
	if err != nil {
		t.Fatalf("leaseUpdateMessage: %v", err)
	}
	encoded, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeMessage(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var lu LeaseUpdatePayload
	if err := json.Unmarshal(decoded.Payload, &lu); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return lu
}

func TestApplyLeaseUpdate_UpsertThenTerminalDelete(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	// Active lease replicated from peer is applied.
	active := sampleLease("192.168.1.50", now, dhcpv4.LeaseStateActive, 5)
	if err := ApplyLeaseUpdate(store, payloadFor(t, active)); err != nil {
		t.Fatalf("apply active: %v", err)
	}
	if got := store.GetByIP(active.IP); got == nil {
		t.Fatal("expected lease present after apply")
	}
	// Seq high-water mark advanced so a local NextSeq won't collide with peer's.
	if next := store.NextSeq(); next <= 5 {
		t.Fatalf("NextSeq = %d, want > 5 (peer seq)", next)
	}

	// A terminal-state update deletes it.
	released := sampleLease("192.168.1.50", now.Add(time.Second), dhcpv4.LeaseStateReleased, 7)
	if err := ApplyLeaseUpdate(store, payloadFor(t, released)); err != nil {
		t.Fatalf("apply released: %v", err)
	}
	if got := store.GetByIP(active.IP); got != nil {
		t.Fatal("expected lease deleted after terminal-state update")
	}
}

func TestApplyLeaseUpdate_LastWriteWins(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	newer := sampleLease("192.168.1.60", now, dhcpv4.LeaseStateActive, 10)
	if err := ApplyLeaseUpdate(store, payloadFor(t, newer)); err != nil {
		t.Fatalf("apply newer: %v", err)
	}

	// A stale update (older LastUpdated) must not overwrite or delete.
	staleDelete := sampleLease("192.168.1.60", now.Add(-time.Minute), dhcpv4.LeaseStateExpired, 3)
	if err := ApplyLeaseUpdate(store, payloadFor(t, staleDelete)); err != nil {
		t.Fatalf("apply stale: %v", err)
	}
	if got := store.GetByIP(newer.IP); got == nil {
		t.Fatal("stale delete should not have removed the newer lease")
	}
}
