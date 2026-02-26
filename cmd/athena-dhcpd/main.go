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
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/api"
	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dbconfig"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dhcp"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dnsproxy"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/ha"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/logging"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
)

func main() {
	configPath := flag.String("config", "/etc/athena-dhcpd/config.toml", "path to configuration file")
	reimport := flag.Bool("reimport", false, "force re-import of TOML config into database (overwrites DB config with TOML)")
	flag.Parse()

	// Load bootstrap configuration (server + api only from TOML)
	bootstrap, err := config.LoadBootstrap(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logger := logging.Setup(bootstrap.Server.LogLevel, os.Stdout)
	logger.Info("athena-dhcpd starting",
		"config", *configPath,
		"interface", bootstrap.Server.Interface,
		"server_id", bootstrap.Server.ServerID)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize lease store (BoltDB)
	store, err := lease.NewStore(bootstrap.Server.LeaseDB)
	if err != nil {
		logger.Error("failed to open lease database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("lease database opened", "path", bootstrap.Server.LeaseDB, "lease_count", store.Count())

	// Initialize config store (dynamic config in BoltDB)
	cfgStore, err := dbconfig.NewStore(store.DB())
	if err != nil {
		logger.Error("failed to initialize config store", "error", err)
		os.Exit(1)
	}

	// Auto-migrate TOML config into database
	if *reimport || !cfgStore.IsV1Imported() {
		if *reimport {
			logger.Info("forced reimport of TOML config into database")
		}
		// Try loading full TOML to check for dynamic config sections
		fullCfg, fullErr := config.Load(*configPath)
		if fullErr == nil && config.HasDynamicConfig(fullCfg) {
			logger.Info("importing TOML config sections to database")
			if err := cfgStore.ImportFromConfig(fullCfg); err != nil {
				logger.Error("failed to import config", "error", err)
				os.Exit(1)
			}
			cfgStore.MarkV1Imported()
			logger.Info("config imported successfully",
				"subnets", len(fullCfg.Subnets))
		} else if fullErr != nil {
			logger.Warn("could not load full TOML for import", "error", fullErr)
		} else {
			logger.Debug("no dynamic config sections in TOML, skipping import")
		}
	}

	// Build full config from bootstrap + DB
	cfg := cfgStore.BuildConfig(bootstrap)
	config.ApplyDynamicDefaults(cfg)

	// Initialize event bus
	bus := events.NewBus(cfg.Hooks.EventBufferSize, logger)
	go bus.Start()
	defer bus.Stop()

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

	// Create and start DHCP server group (one listener per interface)
	serverGroup := dhcp.NewServerGroup(handler, logger)
	if err := serverGroup.Start(ctx, cfg); err != nil {
		logger.Error("failed to start DHCP server", "error", err)
		os.Exit(1)
	}

	// Set server metrics
	metrics.ServerStartTime.SetToCurrentTime()
	metrics.ServerInfo.WithLabelValues("dev").Set(1)

	// Write PID file
	if cfg.Server.PIDFile != "" {
		if err := writePIDFile(cfg.Server.PIDFile); err != nil {
			logger.Warn("failed to write PID file", "path", cfg.Server.PIDFile, "error", err)
		} else {
			defer removePIDFile(cfg.Server.PIDFile)
		}
	}

	// Initialize DNS proxy
	var dnsServer *dnsproxy.Server
	if cfg.DNS.Enabled {
		dnsServer = dnsproxy.NewServer(&cfg.DNS, logger)
		if err := dnsServer.Start(ctx); err != nil {
			logger.Error("failed to start DNS proxy", "error", err)
			// Non-fatal — DHCP still works
		} else {
			// Subscribe to lease events for DNS registration
			dnsEventCh := bus.Subscribe(1000)
			dnsServer.SubscribeToEvents(ctx, dnsEventCh)

			// Register existing leases
			if cfg.DNS.RegisterLeases {
				for _, l := range store.All() {
					if l.State == "active" && l.Hostname != "" {
						dnsServer.RegisterLease(l.Hostname, l.IP)
					}
				}
				logger.Info("DNS proxy loaded existing leases", "zone_records", dnsServer.Zone().Count())
			}
		}
	}

	// Initialize HA peer with config sync (before API so status is available)
	var haPeer *ha.Peer
	var haFSM *ha.FSM
	if cfg.HA.Enabled {
		failoverTimeout, _ := time.ParseDuration(cfg.HA.FailoverTimeout)
		if failoverTimeout == 0 {
			failoverTimeout = 10 * time.Second
		}
		haFSM = ha.NewFSM(cfg.HA.Role, failoverTimeout, bus, logger)
		peer, err := ha.NewPeer(&cfg.HA, haFSM, store, bus, logger)
		if err != nil {
			logger.Error("failed to create HA peer", "error", err)
		} else {
			// Wire config sync: incoming peer config → apply to local DB
			peer.OnConfigSync(func(cs ha.ConfigSyncPayload) {
				if err := cfgStore.ApplyPeerConfig(cs.Section, cs.Data); err != nil {
					logger.Error("failed to apply config from peer",
						"section", cs.Section, "error", err)
				} else {
					logger.Info("applied config from peer", "section", cs.Section)
				}
			})

			// On adjacency formed: if we are the active node, push full config to backup
			peer.OnAdjacencyFormed(func() {
				if !peer.FSM().IsActive() {
					logger.Info("HA adjacency formed, we are standby — waiting for config from primary")
					return
				}
				logger.Info("HA adjacency formed, we are active — pushing full config to peer")
				sections := cfgStore.ExportAllSections()
				if err := peer.SendFullConfigSync(sections); err != nil {
					logger.Error("failed to push full config to peer on adjacency", "error", err)
				} else {
					logger.Info("full config sync to peer complete", "sections", len(sections))
				}
			})

			if err := peer.Start(ctx); err != nil {
				logger.Error("failed to start HA peer", "error", err)
			} else {
				haPeer = peer
				defer haPeer.Stop()
			}
		}
	}

	// Initialize API server
	var apiServer *api.Server
	if cfg.API.Enabled {
		// Flatten pool map for API
		var allPools []*pool.Pool
		for _, subPools := range pools {
			allPools = append(allPools, subPools...)
		}

		// Get conflict table if available
		var conflictTable *conflict.Table
		if detector != nil {
			conflictTable = detector.Table()
		}

		apiOpts := []api.ServerOption{
			api.WithConfigPath(*configPath),
			api.WithConfigStore(cfgStore),
		}
		if dnsServer != nil {
			apiOpts = append(apiOpts, api.WithDNSProxy(dnsServer))
		}
		if haFSM != nil {
			apiOpts = append(apiOpts, api.WithFSM(haFSM))
		}
		if haPeer != nil {
			apiOpts = append(apiOpts, api.WithPeer(haPeer))
		}

		apiServer = api.NewServer(cfg, store, leaseMgr, conflictTable, allPools, bus, logger, apiOpts...)
		go func() {
			if err := apiServer.Start(); err != nil {
				logger.Error("API server failed", "error", err)
			}
		}()
	}

	logger.Info("athena-dhcpd ready",
		"subnets", len(cfg.Subnets),
		"conflict_detection", cfg.ConflictDetection.Enabled,
		"dns_proxy", cfg.DNS.Enabled,
		"api", cfg.API.Enabled,
		"ha", cfg.HA.Enabled)

	// Wire local config changes → send to HA peer
	cfgStore.OnLocalChange(func(section string, data []byte) {
		if haPeer == nil {
			return
		}
		if err := haPeer.SendConfigSync(section, data); err != nil {
			logger.Warn("failed to sync config to HA peer",
				"section", section, "error", err)
		} else {
			logger.Debug("config synced to HA peer", "section", section)
		}
	})

	// Register config change callback — full live reload on every DB config change
	cfgStore.OnChange(func() {
		logger.Info("config changed in database, rebuilding")
		newCfg := cfgStore.BuildConfig(bootstrap)
		config.ApplyDynamicDefaults(newCfg)
		cfg = newCfg

		// Update DHCP handler + lease manager
		leaseMgr.UpdateConfig(cfg)
		handler.UpdateConfig(cfg)

		// Rebuild pools
		newPools, err := initPools(cfg, store)
		if err != nil {
			logger.Error("failed to reinitialize pools after config change", "error", err)
			return
		}
		handler.UpdatePools(newPools)

		// Update API server pool list
		if apiServer != nil {
			var allPools []*pool.Pool
			for _, subPools := range newPools {
				allPools = append(allPools, subPools...)
			}
			apiServer.UpdatePools(allPools)
		}

		// Reload DHCP listeners — add/remove interfaces as needed
		serverGroup.Reload(cfg)

		logger.Info("live config reload complete",
			"subnets", len(cfg.Subnets),
			"conflict_detection", cfg.ConflictDetection.Enabled)
	})

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			logger.Info("received SIGHUP, reloading bootstrap config")
			newBootstrap, err := config.LoadBootstrap(*configPath)
			if err != nil {
				logger.Error("failed to reload bootstrap config", "error", err)
				continue
			}
			bootstrap = newBootstrap
			cfg = cfgStore.BuildConfig(bootstrap)
			config.ApplyDynamicDefaults(cfg)
			leaseMgr.UpdateConfig(cfg)
			handler.UpdateConfig(cfg)
			newPools, err := initPools(cfg, store)
			if err != nil {
				logger.Error("failed to reinitialize pools", "error", err)
				continue
			}
			handler.UpdatePools(newPools)
			serverGroup.Reload(cfg)
			logger.Info("configuration reloaded successfully")

		case syscall.SIGINT, syscall.SIGTERM:
			logger.Info("received shutdown signal", "signal", sig.String())

			// Graceful shutdown with timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			// Cancel the main context to stop all background goroutines
			cancel()

			// Stop API server
			if apiServer != nil {
				apiServer.Stop(shutdownCtx)
			}

			// Stop DNS proxy
			if dnsServer != nil {
				dnsServer.Stop()
			}

			// Stop DHCP server group (stops accepting new packets)
			serverGroup.Stop()

			// Stop event bus (drains remaining events)
			bus.Stop()

			// Close lease store
			store.Close()

			_ = shutdownCtx // used for future API server shutdown

			logger.Info("athena-dhcpd stopped")
			return
		}
	}
}

// writePIDFile writes the current process ID to the given path.
func writePIDFile(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating PID directory %s: %w", dir, err)
		}
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// removePIDFile removes the PID file.
func removePIDFile(path string) {
	os.Remove(path)
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
			p.MatchUserClass = pcfg.MatchUserClass
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

	// Initialize pool metrics after reconciliation
	for _, subPools := range pools {
		for _, p := range subPools {
			p.InitMetrics()
		}
	}

	return pools, nil
}
