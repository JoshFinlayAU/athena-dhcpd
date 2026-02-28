// athena-dhcpd — RFC-compliant DHCPv4 server with built-in high availability.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	nethttp "net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/anomaly"
	"github.com/athena-dhcpd/athena-dhcpd/internal/api"
	"github.com/athena-dhcpd/athena-dhcpd/internal/audit"
	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/conflict"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dbconfig"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dhcp"
	"github.com/athena-dhcpd/athena-dhcpd/internal/dnsproxy"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/fingerprint"
	"github.com/athena-dhcpd/athena-dhcpd/internal/ha"
	"github.com/athena-dhcpd/athena-dhcpd/internal/lease"
	"github.com/athena-dhcpd/athena-dhcpd/internal/logging"
	"github.com/athena-dhcpd/athena-dhcpd/internal/macvendor"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
	"github.com/athena-dhcpd/athena-dhcpd/internal/pool"
	"github.com/athena-dhcpd/athena-dhcpd/internal/portauto"
	"github.com/athena-dhcpd/athena-dhcpd/internal/rogue"
	syslogfwd "github.com/athena-dhcpd/athena-dhcpd/internal/syslog"
	"github.com/athena-dhcpd/athena-dhcpd/internal/topology"
	"github.com/athena-dhcpd/athena-dhcpd/internal/vip"
	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

func main() {
	configPath := flag.String("config", "/etc/athena-dhcpd/config.toml", "path to configuration file")
	debugPort := flag.String("debug-port", "", "enable pprof debug server on this port (e.g. 6060)")
	flag.Parse()

	// Start pprof debug server if requested
	if *debugPort != "" {
		runtime.SetMutexProfileFraction(5)
		runtime.SetBlockProfileRate(1)
		go func() {
			addr := "0.0.0.0:" + *debugPort
			fmt.Fprintf(os.Stderr, "pprof debug server on http://%s/debug/pprof/\n", addr)
			if err := nethttp.ListenAndServe(addr, nil); err != nil {
				fmt.Fprintf(os.Stderr, "pprof server failed: %v\n", err)
			}
		}()
	}

	// SIGUSR1 dumps all goroutine stacks to /tmp/athena-goroutines.txt
	// Works even under 100% CPU since signals are kernel-delivered
	go func() {
		sigUsr1 := make(chan os.Signal, 1)
		signal.Notify(sigUsr1, syscall.SIGUSR1)
		for range sigUsr1 {
			buf := make([]byte, 64*1024*1024) // 64MB
			n := runtime.Stack(buf, true)     // true = all goroutines
			path := "/tmp/athena-goroutines.txt"
			if err := os.WriteFile(path, buf[:n], 0644); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write goroutine dump: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "goroutine dump written to %s (%d bytes)\n", path, n)
			}
		}
	}()

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

	// ──────────────────────────────────────────────────────────────────────
	// Setup wizard gate — if no config has been set up yet, only start the
	// web UI and block until the wizard completes.
	// ──────────────────────────────────────────────────────────────────────
	if !cfgStore.IsSetupComplete() {
		logger.Info("no configuration found — starting setup wizard")

		setupDone := make(chan struct{}, 1)

		cfg := cfgStore.BuildConfig(bootstrap)
		config.ApplyDynamicDefaults(cfg)
		bus := events.NewBus(cfg.Hooks.EventBufferSize, logger)
		go bus.Start()

		setupOpts := []api.ServerOption{
			api.WithConfigPath(*configPath),
			api.WithConfigStore(cfgStore),
			api.WithSetupMode(func() { setupDone <- struct{}{} }),
		}

		setupServer := api.NewServer(cfg, store, nil, nil, nil, bus, logger, setupOpts...)
		go func() {
			if err := setupServer.Start(); err != nil {
				logger.Error("setup API server failed", "error", err)
			}
		}()

		logger.Info("setup wizard available", "listen", cfg.API.Listen)

		// Block until setup completes or a shutdown signal arrives
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-setupDone:
			logger.Info("setup wizard completed — starting full services")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			setupServer.Stop(shutdownCtx)
			shutdownCancel()
			bus.Stop()
			signal.Reset(syscall.SIGINT, syscall.SIGTERM)

			// Reload bootstrap from TOML — the wizard may have written HA config
			newBootstrap, err := config.LoadBootstrap(*configPath)
			if err != nil {
				logger.Warn("failed to reload bootstrap after setup", "error", err)
			} else {
				bootstrap = newBootstrap
			}
			// Fall through to normal startup below
		case sig := <-sigCh:
			logger.Info("received shutdown signal during setup", "signal", sig.String())
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			setupServer.Stop(shutdownCtx)
			shutdownCancel()
			bus.Stop()
			store.Close()
			return
		}
	}

	// ──────────────────────────────────────────────────────────────────────
	// HA Secondary gate — if we are a secondary node, connect to primary
	// and wait for config sync BEFORE starting any services.
	// The secondary must not serve DHCP or run detection with empty config.
	// ──────────────────────────────────────────────────────────────────────
	var earlyHAPeer *ha.Peer
	var earlyHAFSM *ha.FSM
	var earlyBus *events.Bus

	if bootstrap.HA.Enabled && bootstrap.HA.Role == "secondary" {
		logger.Info("secondary node — connecting to primary before starting services",
			"peer", bootstrap.HA.PeerAddress,
			"listen", bootstrap.HA.ListenAddress)

		earlyBus = events.NewBus(10000, logger)
		go earlyBus.Start()

		failoverTimeout, _ := time.ParseDuration(bootstrap.HA.FailoverTimeout)
		if failoverTimeout == 0 {
			failoverTimeout = 10 * time.Second
		}
		earlyHAFSM = ha.NewFSM(bootstrap.HA.Role, failoverTimeout, earlyBus, logger)
		peer, err := ha.NewPeer(&bootstrap.HA, earlyHAFSM, store, earlyBus, logger)
		if err != nil {
			logger.Error("failed to create HA peer", "error", err)
			os.Exit(1)
		}

		configReady := make(chan struct{}, 1)

		peer.OnConfigSync(func(cs ha.ConfigSyncPayload) {
			if err := cfgStore.ApplyPeerConfig(cs.Section, cs.Data); err != nil {
				logger.Error("failed to apply config from peer",
					"section", cs.Section, "error", err)
			} else {
				logger.Info("applied config from peer", "section", cs.Section)
				select {
				case configReady <- struct{}{}:
				default:
				}
			}
		})

		peer.OnAdjacencyFormed(func() {
			if !peer.FSM().IsActive() {
				logger.Info("HA adjacency formed — waiting for config from primary")
				return
			}
			logger.Info("HA adjacency formed, we are active — pushing config to peer")
			sections := cfgStore.ExportAllSections()
			if err := peer.SendFullConfigSync(sections); err != nil {
				logger.Error("failed to push config to peer", "error", err)
			}
		})

		if err := peer.Start(ctx); err != nil {
			logger.Error("failed to start HA peer", "error", err)
			os.Exit(1)
		}
		earlyHAPeer = peer

		// Start minimal API for monitoring while waiting
		minCfg := cfgStore.BuildConfig(bootstrap)
		config.ApplyDynamicDefaults(minCfg)
		apiOpts := []api.ServerOption{
			api.WithConfigPath(*configPath),
			api.WithConfigStore(cfgStore),
			api.WithFSM(earlyHAFSM),
			api.WithPeer(earlyHAPeer),
		}
		minAPI := api.NewServer(minCfg, store, nil, nil, nil, earlyBus, logger, apiOpts...)
		go func() {
			if err := minAPI.Start(); err != nil {
				logger.Error("API server failed", "error", err)
			}
		}()

		logger.Info("secondary waiting for config from primary...",
			"api", minCfg.API.Listen)

		// Block until config sync or shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-configReady:
			// Brief pause to let debounced config sections finish arriving
			time.Sleep(500 * time.Millisecond)
			logger.Info("config received from primary — initializing standby")
			signal.Reset(syscall.SIGINT, syscall.SIGTERM)

			// Stop minimal API — full API starts below
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			minAPI.Stop(shutdownCtx)
			shutdownCancel()

			if !cfgStore.IsSetupComplete() {
				cfgStore.MarkSetupComplete()
			}

		case sig := <-sigCh:
			logger.Info("shutdown during config wait", "signal", sig.String())
			earlyHAPeer.Stop()
			earlyBus.Stop()
			store.Close()
			return
		}

		// ──────────────────────────────────────────────────────────────
		// Secondary lifecycle — standby until failover
		// Services (DHCP, conflict, rogue, DNS) start ONLY when FSM
		// transitions to ACTIVE. Until then we just sync leases/config.
		// ──────────────────────────────────────────────────────────────

		cfg := cfgStore.BuildConfig(bootstrap)
		config.ApplyDynamicDefaults(cfg)

		// Lease manager needed for receiving lease syncs from primary
		leaseMgr := lease.NewManager(store, cfg, earlyBus, logger)
		leaseMgr.StartGC(ctx, 60*time.Second)

		// Pools + handler ready but NOT serving yet
		pools, err := initPools(cfg, store)
		if err != nil {
			logger.Error("failed to initialize pools", "error", err)
			os.Exit(1)
		}
		handler := dhcp.NewHandler(cfg, leaseMgr, pools, nil, earlyBus, logger)
		handler.SetHA(earlyHAFSM)

		// Server group created but NOT started — waits for failover
		serverGroup := dhcp.NewServerGroup(handler, logger)

		// Mutable service state protected by mutex — started/stopped on failover
		var (
			svcMu      sync.Mutex
			svcRunning bool
			svcDNS     *dnsproxy.Server
			svcRogue   *rogue.Detector
		)

		startActiveServices := func() {
			svcMu.Lock()
			defer svcMu.Unlock()
			if svcRunning {
				return
			}
			svcRunning = true
			logger.Warn("FAILOVER: secondary becoming ACTIVE — starting services")

			if err := serverGroup.Start(ctx, cfg); err != nil {
				logger.Error("failed to start DHCP server on failover", "error", err)
			}

			if cfg.ConflictDetection.Enabled {
				det, detErr := initConflictDetection(cfg, store, earlyBus, logger)
				if detErr != nil {
					logger.Error("failed to start conflict detection", "error", detErr)
				} else {
					handler.UpdateDetector(det)
				}
			}

			var ownIPs []net.IP
			if ip := cfg.ServerIP(); ip != nil {
				ownIPs = append(ownIPs, ip)
			}
			if iface, ifErr := net.InterfaceByName(cfg.Server.Interface); ifErr == nil {
				if addrs, aErr := iface.Addrs(); aErr == nil {
					for _, a := range addrs {
						if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
							ownIPs = append(ownIPs, ipn.IP.To4())
						}
					}
				}
			}
			rd, rdErr := rogue.NewDetector(store.DB(), earlyBus, ownIPs, logger)
			if rdErr != nil {
				logger.Warn("failed to start rogue detector", "error", rdErr)
			} else {
				svcRogue = rd
				svcRogue.StartProbing(ctx, rogue.ProbeConfig{
					Interface: cfg.Server.Interface,
					Interval:  5 * time.Minute,
					Timeout:   3 * time.Second,
				})
			}

			if cfg.DNS.Enabled {
				svcDNS = dnsproxy.NewServer(&cfg.DNS, logger)
				if dnsErr := svcDNS.Start(ctx); dnsErr != nil {
					logger.Error("failed to start DNS proxy on failover", "error", dnsErr)
					svcDNS = nil
				}
			}

			metrics.ServerStartTime.SetToCurrentTime()
			logger.Warn("secondary now ACTIVE — all services running")
		}

		stopActiveServices := func() {
			svcMu.Lock()
			defer svcMu.Unlock()
			if !svcRunning {
				return
			}
			svcRunning = false
			logger.Warn("returning to STANDBY — stopping active services")
			serverGroup.Stop()
			if svcDNS != nil {
				svcDNS.Stop()
				svcDNS = nil
			}
			if svcRogue != nil {
				svcRogue.Stop()
				svcRogue = nil
			}
			handler.UpdateDetector(nil)
		}

		// FSM callback — start/stop services on failover transitions
		earlyHAFSM.OnStateChange(func(oldState, newState dhcpv4.HAState) {
			isNowActive := newState == dhcpv4.HAStateActive || newState == dhcpv4.HAStatePartnerDown
			wasActive := oldState == dhcpv4.HAStateActive || oldState == dhcpv4.HAStatePartnerDown
			if isNowActive && !wasActive {
				go startActiveServices()
			} else if !isNowActive && wasActive {
				go stopActiveServices()
			}
		})

		// Start full API server for monitoring
		var allPools []*pool.Pool
		for _, subPools := range pools {
			allPools = append(allPools, subPools...)
		}
		apiOpts = []api.ServerOption{
			api.WithConfigPath(*configPath),
			api.WithConfigStore(cfgStore),
			api.WithFSM(earlyHAFSM),
			api.WithPeer(earlyHAPeer),
		}
		apiServer := api.NewServer(cfg, store, leaseMgr, nil, allPools, earlyBus, logger, apiOpts...)
		go func() {
			if err := apiServer.Start(); err != nil {
				logger.Error("API server failed", "error", err)
			}
		}()

		// Wire config change callbacks
		cfgStore.OnLocalChange(func(section string, data []byte) {
			if err := earlyHAPeer.SendConfigSync(section, data); err != nil {
				logger.Warn("failed to sync config to HA peer",
					"section", section, "error", err)
			}
		})

		cfgStore.OnChange(func() {
			logger.Info("config changed in database, rebuilding")
			newCfg := cfgStore.BuildConfig(bootstrap)
			config.ApplyDynamicDefaults(newCfg)
			cfg = newCfg
			leaseMgr.UpdateConfig(cfg)
			handler.UpdateConfig(cfg)
			newPools, poolErr := initPools(cfg, store)
			if poolErr != nil {
				logger.Error("failed to reinitialize pools", "error", poolErr)
				return
			}
			handler.UpdatePools(newPools)
			if apiServer != nil {
				apiServer.UpdateConfig(cfg)
				var ap []*pool.Pool
				for _, sp := range newPools {
					ap = append(ap, sp...)
				}
				apiServer.UpdatePools(ap)
			}
		})

		if cfg.Server.PIDFile != "" {
			if err := writePIDFile(cfg.Server.PIDFile); err != nil {
				logger.Warn("failed to write PID file", "error", err)
			} else {
				defer removePIDFile(cfg.Server.PIDFile)
			}
		}

		logger.Info("athena-dhcpd secondary ready (standby)",
			"subnets", len(cfg.Subnets),
			"ha_state", string(earlyHAFSM.State()))

		// Secondary signal handling loop
		sigCh2 := make(chan os.Signal, 1)
		signal.Notify(sigCh2, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

		for {
			sig := <-sigCh2
			switch sig {
			case syscall.SIGHUP:
				logger.Info("received SIGHUP, reloading bootstrap config")
				newBootstrap, hupErr := config.LoadBootstrap(*configPath)
				if hupErr != nil {
					logger.Error("failed to reload bootstrap config", "error", hupErr)
					continue
				}
				bootstrap = newBootstrap
				cfg = cfgStore.BuildConfig(bootstrap)
				config.ApplyDynamicDefaults(cfg)
				leaseMgr.UpdateConfig(cfg)
				handler.UpdateConfig(cfg)
				newPools, poolErr := initPools(cfg, store)
				if poolErr != nil {
					logger.Error("failed to reinitialize pools", "error", poolErr)
					continue
				}
				handler.UpdatePools(newPools)
				logger.Info("configuration reloaded successfully")

			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("received shutdown signal", "signal", sig.String())
				cancel()
				stopActiveServices()
				sdCtx, sdCancel := context.WithTimeout(context.Background(), 10*time.Second)
				apiServer.Stop(sdCtx)
				sdCancel()
				earlyHAPeer.Stop()
				earlyBus.Stop()
				store.Close()
				logger.Info("athena-dhcpd stopped")
				return
			}
		}
	}

	// ──────────────────────────────────────────────────────────────────────
	// Normal startup — full service initialization
	// ──────────────────────────────────────────────────────────────────────

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

	// Initialize rogue DHCP server detection with active probing
	var rogueDetector *rogue.Detector
	{
		// Collect our own IPs so we don't flag ourselves
		var ownIPs []net.IP
		if ip := cfg.ServerIP(); ip != nil {
			ownIPs = append(ownIPs, ip)
		}
		if iface, err := net.InterfaceByName(cfg.Server.Interface); err == nil {
			if addrs, err := iface.Addrs(); err == nil {
				for _, a := range addrs {
					if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
						ownIPs = append(ownIPs, ipn.IP.To4())
					}
				}
			}
		}
		rd, err := rogue.NewDetector(store.DB(), bus, ownIPs, logger)
		if err != nil {
			logger.Warn("failed to initialize rogue detector", "error", err)
		} else {
			rogueDetector = rd
			rogueDetector.StartProbing(ctx, rogue.ProbeConfig{
				Interface: cfg.Server.Interface,
				Interval:  5 * time.Minute,
				Timeout:   3 * time.Second,
			})
			logger.Info("rogue DHCP detection enabled", "own_ips", len(ownIPs))
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
		// Primary / standalone path — start HA now
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

			// Wire HA state into DHCP handler so standby node drops packets
			handler.SetHA(haFSM)

			if err := peer.Start(ctx); err != nil {
				logger.Error("failed to start HA peer", "error", err)
			} else {
				haPeer = peer
				defer haPeer.Stop()
			}
		}
	}

	// Initialize audit log
	auditLog, err := audit.NewLog(store.DB(), bus, cfg.Server.ServerID, logger)
	if err != nil {
		logger.Warn("failed to initialize audit log", "error", err)
	} else {
		go auditLog.Start()
		defer auditLog.Stop()
	}

	// Initialize anomaly detector
	anomalyDet := anomaly.NewDetector(bus, anomaly.DefaultConfig(), logger)
	go anomalyDet.Start()
	defer anomalyDet.Stop()

	// Initialize syslog forwarder if configured
	var syslogForwarder *syslogfwd.Forwarder
	if cfg.Syslog.Enabled && cfg.Syslog.Address != "" {
		syslogCfg := syslogfwd.Config{
			Address:  cfg.Syslog.Address,
			Protocol: cfg.Syslog.Protocol,
			Facility: cfg.Syslog.Facility,
			Tag:      cfg.Syslog.Tag,
		}
		syslogForwarder = syslogfwd.NewForwarder(syslogCfg, bus, logger)
		if err := syslogForwarder.Start(); err != nil {
			logger.Warn("failed to start syslog forwarder", "error", err)
			syslogForwarder = nil
		} else {
			defer syslogForwarder.Stop()
		}
	}

	// Initialize fingerprint store
	fpStore, err := fingerprint.NewStore(store.DB(), logger)
	if err != nil {
		logger.Warn("failed to initialize fingerprint store", "error", err)
	} else {
		logger.Info("fingerprint store initialized")

		// Set up Fingerbank API client if configured
		if cfg.Fingerprint.FingerbankAPI != "" {
			fb := fingerprint.NewFingerbankClient(cfg.Fingerprint.FingerbankAPI, cfg.Fingerprint.FingerbankURL, logger)
			if fb != nil {
				fpStore.SetFingerbank(fb)
				logger.Info("fingerbank API client configured")
			}
		}

		// Wire fingerprint store into DHCP handler
		handler.SetFingerprintStore(fpStore)
	}

	// Initialize topology map
	topoMap, err := topology.NewMap(store.DB(), logger)
	if err != nil {
		logger.Warn("failed to initialize topology map", "error", err)
	} else {
		logger.Info("topology map initialized")
	}

	// Initialize port automation engine
	portAutoEngine := portauto.NewEngine(logger)
	if rulesJSON := cfgStore.PortAutoRules(); rulesJSON != nil {
		var rules []portauto.Rule
		if err := json.Unmarshal(rulesJSON, &rules); err != nil {
			logger.Warn("failed to load portauto rules from db", "error", err)
		} else if err := portAutoEngine.SetRules(rules); err != nil {
			logger.Warn("failed to apply portauto rules from db", "error", err)
		} else {
			logger.Info("port automation rules loaded", "count", len(rules))
		}
	}

	// Initialize MAC vendor database
	macVendorDB := macvendor.NewDB(logger)

	// Initialize API server (always on — essential service)
	var allPools []*pool.Pool
	for _, subPools := range pools {
		allPools = append(allPools, subPools...)
	}

	var conflictTable *conflict.Table
	if detector != nil {
		conflictTable = detector.Table()
	}

	apiOpts := []api.ServerOption{
		api.WithConfigPath(*configPath),
		api.WithConfigStore(cfgStore),
		api.WithAnomalyDetector(anomalyDet),
		api.WithMACVendorDB(macVendorDB),
	}
	if auditLog != nil {
		apiOpts = append(apiOpts, api.WithAuditLog(auditLog))
	}
	if fpStore != nil {
		apiOpts = append(apiOpts, api.WithFingerprintStore(fpStore))
	}
	if topoMap != nil {
		apiOpts = append(apiOpts, api.WithTopologyMap(topoMap))
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
	if rogueDetector != nil {
		apiOpts = append(apiOpts, api.WithRogueDetector(rogueDetector))
	}
	if portAutoEngine != nil {
		apiOpts = append(apiOpts, api.WithPortAutoEngine(portAutoEngine))
	}

	// Initialize floating VIP group — always create one (even empty)
	// so hot-reload works when VIPs are added via API without restart.
	var vipEntries []vip.Entry
	if vipData := cfgStore.VIPs(); vipData != nil {
		entries, err := vip.ParseEntries(vipData)
		if err != nil {
			logger.Warn("failed to parse VIP entries from database", "error", err)
		} else {
			vipEntries = entries
		}
	}
	vipGroup, err := vip.NewGroup(vipEntries, logger)
	if err != nil {
		logger.Warn("failed to create VIP group", "error", err)
		vipGroup, _ = vip.NewGroup(nil, logger)
	}
	defer vipGroup.ReleaseAll()

	// If we already have VIPs and are active, acquire now
	if len(vipEntries) > 0 && haFSM != nil && haFSM.IsActive() {
		vipGroup.AcquireAll()
	}

	// Wire VIP acquire/release to HA state transitions
	if haFSM != nil {
		haFSM.OnStateChange(func(oldState, newState dhcpv4.HAState) {
			isNowActive := newState == dhcpv4.HAStateActive || newState == dhcpv4.HAStatePartnerDown
			wasActive := oldState == dhcpv4.HAStateActive || oldState == dhcpv4.HAStatePartnerDown
			if isNowActive && !wasActive {
				logger.Info("HA became active — acquiring VIPs")
				vipGroup.AcquireAll()
			} else if !isNowActive && wasActive {
				logger.Info("HA became standby — releasing VIPs")
				vipGroup.ReleaseAll()
			}
		})
	}

	apiOpts = append(apiOpts, api.WithVIPGroup(vipGroup))

	apiServer := api.NewServer(cfg, store, leaseMgr, conflictTable, allPools, bus, logger, apiOpts...)
	apiLn, err := apiServer.Listen()
	if err != nil {
		logger.Error("FATAL: API server failed to start", "error", err)
		os.Exit(1)
	} else {
		logger.Info("API server started", "listen", apiLn.Addr().String(), "config", cfg.API.Listen)
	}

	go func() {
		if err := apiServer.Serve(apiLn); err != nil {
			logger.Error("API server failed", "error", err)
		}
	}()

	logger.Info("athena-dhcpd ready",
		"subnets", len(cfg.Subnets),
		"conflict_detection", cfg.ConflictDetection.Enabled,
		"dns_proxy", cfg.DNS.Enabled,
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

		// Update API server config + pool list
		if apiServer != nil {
			apiServer.UpdateConfig(cfg)
			var allPools []*pool.Pool
			for _, subPools := range newPools {
				allPools = append(allPools, subPools...)
			}
			apiServer.UpdatePools(allPools)
		}

		// Reload DNS proxy config (filter lists, forwarders, zone overrides)
		if dnsServer != nil {
			dnsServer.UpdateConfig(&cfg.DNS)
		}

		// Reload Fingerbank API client if API key changed
		if fpStore != nil {
			if cfg.Fingerprint.FingerbankAPI != "" {
				fb := fingerprint.NewFingerbankClient(cfg.Fingerprint.FingerbankAPI, cfg.Fingerprint.FingerbankURL, logger)
				fpStore.SetFingerbank(fb)
			} else {
				fpStore.SetFingerbank(nil)
			}
		}

		// Reload syslog forwarder
		if cfg.Syslog.Enabled && cfg.Syslog.Address != "" {
			if syslogForwarder == nil {
				syslogCfg := syslogfwd.Config{
					Address:  cfg.Syslog.Address,
					Protocol: cfg.Syslog.Protocol,
					Facility: cfg.Syslog.Facility,
					Tag:      cfg.Syslog.Tag,
				}
				syslogForwarder = syslogfwd.NewForwarder(syslogCfg, bus, logger)
				if err := syslogForwarder.Start(); err != nil {
					logger.Warn("failed to start syslog forwarder after config change", "error", err)
					syslogForwarder = nil
				}
			}
		} else if syslogForwarder != nil {
			syslogForwarder.Stop()
			syslogForwarder = nil
			logger.Info("syslog forwarder stopped (disabled in config)")
		}

		// Reload port automation rules from DB
		if portAutoEngine != nil {
			if rulesJSON := cfgStore.PortAutoRules(); rulesJSON != nil {
				var rules []portauto.Rule
				if err := json.Unmarshal(rulesJSON, &rules); err != nil {
					logger.Warn("failed to reload portauto rules", "error", err)
				} else if err := portAutoEngine.SetRules(rules); err != nil {
					logger.Warn("failed to apply portauto rules after config change", "error", err)
				}
			}
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
