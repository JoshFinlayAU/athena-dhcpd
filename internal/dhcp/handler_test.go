package dhcp

import (
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
)

// TestHandlerConcurrentHotSwap exercises the RWMutex guarding the handler's
// hot-swappable fields: many goroutines read through the accessors while others
// replace config/pools/detector/HA concurrently. Run with -race to catch a
// regression where a field is read or written without the lock.
func TestHandlerConcurrentHotSwap(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mkCfg := func(serverID string) *config.Config {
		return &config.Config{
			Server: config.ServerConfig{Interface: "lo", ServerID: serverID},
			Subnets: []config.SubnetConfig{
				{Network: "192.168.1.0/24", Interface: "lo"},
			},
		}
	}

	h := NewHandler(mkCfg("192.168.1.1"), nil, map[string][]*pool.Pool{}, nil, nil, logger)

	var wg sync.WaitGroup
	const workers = 8
	const iters = 500

	// Readers — touch every guarded field through its accessor.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = h.config()
				_ = h.poolMap()
				_ = h.activeDetector()
				_ = h.serverIdentity()
				_ = h.haChecker()
				_ = h.fingerprints()
				_, _ = h.findSubnetForIP(h.serverIdentity())
			}
		}()
	}

	// Writers — hot-swap the guarded fields.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				h.UpdateConfig(mkCfg("192.168.1.2"))
				h.UpdatePools(map[string][]*pool.Pool{"x": nil})
				h.UpdateDetector(nil)
				h.SetHA(nil)
				h.SetFingerprintStore(nil)
			}
		}(i)
	}

	wg.Wait()
}
