package ha

import (
	"fmt"
	"net"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// ApplyLeaseUpdate applies a lease update received from the HA peer to the local
// store. Active/offered leases are upserted; terminal states (released, expired,
// declined) delete the local lease. Conflict resolution is last-write-wins on
// the update timestamp, handled by the store. This path never re-replicates.
func ApplyLeaseUpdate(store *lease.Store, lu LeaseUpdatePayload) error {
	ip := net.ParseIP(lu.IP)
	if ip == nil {
		return fmt.Errorf("invalid IP in lease update: %q", lu.IP)
	}
	updated := time.Unix(0, lu.Updated)

	switch dhcpv4.LeaseState(lu.State) {
	case dhcpv4.LeaseStateReleased, dhcpv4.LeaseStateExpired, dhcpv4.LeaseStateDeclined:
		_, err := store.ApplyRemoteDelete(ip, updated)
		return err
	}

	mac, err := net.ParseMAC(lu.MAC)
	if err != nil {
		return fmt.Errorf("invalid MAC in lease update for %s: %w", lu.IP, err)
	}

	l := &lease.Lease{
		IP:          ip,
		MAC:         mac,
		ClientID:    lu.ClientID,
		Hostname:    lu.Hostname,
		Subnet:      lu.Subnet,
		Pool:        lu.Pool,
		State:       dhcpv4.LeaseState(lu.State),
		Start:       time.Unix(lu.Start, 0),
		Expiry:      time.Unix(lu.Expiry, 0),
		LastUpdated: updated,
		UpdateSeq:   lu.Seq,
	}
	_, err = store.ApplyRemote(l)
	return err
}
