package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// ServerGroup manages multiple DHCP listeners — one per unique interface
// declared in the subnet configs. All listeners share the same Handler.
type ServerGroup struct {
	handler     *Handler
	logger      *slog.Logger
	defaultAddr string

	mu      sync.Mutex
	servers map[string]*Server // interface name → server
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewServerGroup creates a new server group with a shared handler.
func NewServerGroup(handler *Handler, logger *slog.Logger) *ServerGroup {
	return &ServerGroup{
		handler:     handler,
		logger:      logger,
		defaultAddr: fmt.Sprintf(":%d", dhcpv4.ServerPort),
		servers:     make(map[string]*Server),
	}
}

// Start creates listeners for all unique interfaces in the config and begins serving.
func (g *ServerGroup) Start(ctx context.Context, cfg *config.Config) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.ctx, g.cancel = context.WithCancel(ctx)

	interfaces := g.collectInterfaces(cfg)
	if len(interfaces) == 0 {
		// Fallback: use the default server.interface from bootstrap config
		interfaces = []string{cfg.Server.Interface}
	}

	for _, iface := range interfaces {
		if err := g.startListener(iface); err != nil {
			return err
		}
	}

	return nil
}

// Reload updates the set of active listeners based on current config.
// Adds new interface listeners and removes stale ones. Existing ones stay up.
func (g *ServerGroup) Reload(cfg *config.Config) {
	g.mu.Lock()
	defer g.mu.Unlock()

	wanted := make(map[string]bool)
	for _, iface := range g.collectInterfaces(cfg) {
		wanted[iface] = true
	}
	// Fallback if no interfaces declared
	if len(wanted) == 0 {
		wanted[cfg.Server.Interface] = true
	}

	// Start new listeners
	for iface := range wanted {
		if _, exists := g.servers[iface]; !exists {
			if err := g.startListener(iface); err != nil {
				g.logger.Error("failed to start DHCP listener on new interface",
					"interface", iface, "error", err)
			}
		}
	}

	// Stop removed listeners
	for iface, srv := range g.servers {
		if !wanted[iface] {
			g.logger.Info("stopping DHCP listener for removed interface", "interface", iface)
			srv.Stop()
			delete(g.servers, iface)
		}
	}
}

// Stop shuts down all listeners.
func (g *ServerGroup) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.cancel != nil {
		g.cancel()
	}
	for iface, srv := range g.servers {
		srv.Stop()
		delete(g.servers, iface)
	}
}

// Handler returns the shared packet handler.
func (g *ServerGroup) Handler() *Handler {
	return g.handler
}

// startListener creates and starts a single DHCP listener for the given interface.
// Caller must hold g.mu.
func (g *ServerGroup) startListener(iface string) error {
	srv := NewServer(g.handler, iface, g.defaultAddr, g.logger)
	if err := srv.Start(g.ctx); err != nil {
		return fmt.Errorf("starting DHCP listener on %s: %w", iface, err)
	}
	g.servers[iface] = srv
	g.logger.Info("DHCP listener started", "interface", iface)
	return nil
}

// collectInterfaces returns deduplicated interface names from subnet configs.
func (g *ServerGroup) collectInterfaces(cfg *config.Config) []string {
	seen := make(map[string]bool)
	var result []string
	for _, sub := range cfg.Subnets {
		if sub.Interface != "" && !seen[sub.Interface] {
			seen[sub.Interface] = true
			result = append(result, sub.Interface)
		}
	}
	return result
}
