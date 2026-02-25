// athena-dhcpd — RFC-compliant DHCPv4 server with built-in high availability.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dhcp"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/logging"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
)

func main() {
	configPath := flag.String("config", "/etc/athena-dhcpd/config.toml", "path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logger := logging.Setup(cfg.Server.LogLevel, os.Stdout)
	logger.Info("athena-dhcpd starting",
		"config", *configPath,
		"interface", cfg.Server.Interface,
		"server_id", cfg.Server.ServerID)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize event bus
	bus := events.NewBus(cfg.Hooks.EventBufferSize, logger)
	go bus.Start()
	defer bus.Stop()

	// Initialize lease store (BoltDB)
	store, err := lease.NewStore(cfg.Server.LeaseDB)
	if err != nil {
		logger.Error("failed to open lease database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("lease database opened", "path", cfg.Server.LeaseDB, "lease_count", store.Count())

	// Initialize lease manager
	leaseMgr := lease.NewManager(store, cfg, bus, logger)

	// Start lease GC
	leaseMgr.StartGC(ctx, 60*time.Second)

	// Initialize conflict detection
	var detector *conflict.Detector
	if cfg.ConflictDetection.Enabled {
		detector, err = initConflictDetection(cfg, store, bus, logger)
		if err != nil {
			logger.Error("failed to initialize conflict detection", "error", err)
			// Continue without conflict detection — reduced safety
		}
	}

	// Initialize pools from config
	pools, err := initPools(cfg, store)
	if err != nil {
		logger.Error("failed to initialize pools", "error", err)
		os.Exit(1)
	}

	// Create DHCP handler
	handler := dhcp.NewHandler(cfg, leaseMgr, pools, detector, bus, logger)

	// Create and start DHCP server
	server := dhcp.NewServer(handler, cfg.Server.Interface, fmt.Sprintf(":%d", 67), logger)
	if err := server.Start(ctx); err != nil {
		logger.Error("failed to start DHCP server", "error", err)
		os.Exit(1)
	}

	logger.Info("athena-dhcpd ready",
		"subnets", len(cfg.Subnets),
		"conflict_detection", cfg.ConflictDetection.Enabled)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			logger.Info("received SIGHUP, reloading configuration")
			newCfg, err := config.Load(*configPath)
			if err != nil {
				logger.Error("failed to reload config", "error", err)
				continue
			}
			cfg = newCfg
			leaseMgr.UpdateConfig(cfg)
			handler.UpdateConfig(cfg)
			newPools, err := initPools(cfg, store)
			if err != nil {
				logger.Error("failed to reinitialize pools", "error", err)
				continue
			}
			handler.UpdatePools(newPools)
			logger.Info("configuration reloaded successfully")

		case syscall.SIGINT, syscall.SIGTERM:
			logger.Info("received shutdown signal", "signal", sig.String())
			cancel()
			server.Stop()
			logger.Info("athena-dhcpd stopped")
			return
		}
	}
}

// initConflictDetection sets up the conflict detector with ARP and ICMP probers.
func initConflictDetection(cfg *config.Config, store *lease.Store, bus *events.Bus, logger *slog.Logger) (*conflict.Detector, error) {
	probeTimeout, err := time.ParseDuration(cfg.ConflictDetection.ProbeTimeout)
	if err != nil {
		probeTimeout = 500 * time.Millisecond
	}
	holdTime, err := time.ParseDuration(cfg.ConflictDetection.ConflictHoldTime)
	if err != nil {
		holdTime = time.Hour
	}
	cacheTTL, err := time.ParseDuration(cfg.ConflictDetection.ProbeCacheTTL)
	if err != nil {
		cacheTTL = 10 * time.Second
	}

	// Initialize conflict table
	table, err := conflict.NewTable(store.DB(), holdTime, cfg.ConflictDetection.MaxConflictCount)
	if err != nil {
		return nil, fmt.Errorf("initializing conflict table: %w", err)
	}

	// Initialize ARP prober
	arpProber, err := conflict.NewARPProber(cfg.Server.Interface, logger)
	if err != nil {
		logger.Warn("ARP prober initialization failed — ARP conflict detection disabled",
			"error", err)
	}

	// Initialize ICMP prober
	icmpProber, err := conflict.NewICMPProber(logger)
	if err != nil {
		logger.Warn("ICMP prober initialization failed — ICMP conflict detection disabled",
			"error", err)
	}

	detectorCfg := conflict.DetectorConfig{
		ProbeTimeout:     probeTimeout,
		MaxProbes:        cfg.ConflictDetection.MaxProbesPerDiscover,
		Strategy:         cfg.ConflictDetection.ProbeStrategy,
		ParallelCount:    cfg.ConflictDetection.ParallelProbeCount,
		HoldTime:         holdTime,
		MaxConflictCount: cfg.ConflictDetection.MaxConflictCount,
		CacheTTL:         cacheTTL,
		SendGratuitous:   cfg.ConflictDetection.SendGratuitousARP,
		ICMPFallback:     cfg.ConflictDetection.ICMPFallback,
	}

	detector := conflict.NewDetector(arpProber, icmpProber, table, bus, logger, detectorCfg)

	activeConflicts := table.Count()
	permanentConflicts := table.PermanentCount()
	logger.Info("conflict detection initialized",
		"strategy", cfg.ConflictDetection.ProbeStrategy,
		"probe_timeout", probeTimeout.String(),
		"arp_available", arpProber != nil && arpProber.Available(),
		"icmp_available", icmpProber != nil && icmpProber.Available(),
		"active_conflicts", activeConflicts,
		"permanent_conflicts", permanentConflicts)

	return detector, nil
}

// initPools creates pool objects from the config and reconciles with existing leases.
func initPools(cfg *config.Config, store *lease.Store) (map[string][]*pool.Pool, error) {
	pools := make(map[string][]*pool.Pool)

	for _, sub := range cfg.Subnets {
		_, network, err := net.ParseCIDR(sub.Network)
		if err != nil {
			return nil, fmt.Errorf("parsing subnet %s: %w", sub.Network, err)
		}

		var subPools []*pool.Pool
		for j, pcfg := range sub.Pools {
			start := net.ParseIP(pcfg.RangeStart)
			end := net.ParseIP(pcfg.RangeEnd)
			name := fmt.Sprintf("%s-pool-%d", sub.Network, j)

			p, err := pool.NewPool(name, start, end, network)
			if err != nil {
				return nil, fmt.Errorf("creating pool %s: %w", name, err)
			}

			p.MatchCircuitID = pcfg.MatchCircuitID
			p.MatchRemoteID = pcfg.MatchRemoteID
			p.MatchVendorClass = pcfg.MatchVendorClass
			p.LeaseTime = pcfg.LeaseTime

			subPools = append(subPools, p)
		}

		pools[sub.Network] = subPools
	}

	// Reconcile existing leases with pools — mark allocated IPs
	allLeases := store.All()
	for _, l := range allLeases {
		if l.State != "active" && l.State != "offered" {
			continue
		}
		for _, subPools := range pools {
			for _, p := range subPools {
				if p.Contains(l.IP) {
					p.AllocateSpecific(l.IP)
					break
				}
			}
		}
	}

	return pools, nil
}
