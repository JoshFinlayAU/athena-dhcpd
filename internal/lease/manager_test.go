package lease

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// TestExpireLeasesReleasesPool verifies that expiring a lease returns its IP to
// the pool via the registered releaser — otherwise pool bits leak until restart.
func TestExpireLeasesReleasesPool(t *testing.T) {
	store := newTestStore(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus(16, logger)

	m := &Manager{store: store, bus: bus, logger: logger}

	var released []string
	m.SetPoolReleaser(func(ip net.IP) {
		released = append(released, ip.String())
	})

	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	expired := &Lease{
		IP:          net.ParseIP("192.168.1.10"),
		MAC:         mac,
		Subnet:      "192.168.1.0/24",
		State:       dhcpv4.LeaseStateActive,
		Start:       time.Now().Add(-2 * time.Hour),
		Expiry:      time.Now().Add(-time.Hour), // already expired
		LastUpdated: time.Now().Add(-time.Hour),
	}
	if err := store.Put(expired); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if n := m.ExpireLeases(); n != 1 {
		t.Fatalf("ExpireLeases = %d, want 1", n)
	}
	if len(released) != 1 || released[0] != "192.168.1.10" {
		t.Fatalf("pool releaser got %v, want [192.168.1.10]", released)
	}
	if store.GetByIP(expired.IP) != nil {
		t.Fatal("expired lease should be deleted from store")
	}
}
