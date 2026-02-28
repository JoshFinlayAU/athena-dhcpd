# Architecture

how the code is organized and why. if you're planning to contribute or just want to understand what's going on under the hood

## the big picture

```
client sends DHCPDISCOVER
    │
    ▼
┌─────────────────┐
│  dhcp.Server    │  UDP listener on port 67
│  (server.go)    │  receives raw packets, dispatches to handler
└────────┬────────┘
         │
         ▼
┌───────────────────────────────────────┐
│  dhcp.Handler                         │
│  DORA cycle + Decline, Release, Inform│
│  + fingerprint extraction on DISCOVER │
└──┬───┬───┬──────┬──────────────────────┘
   │   │   │      │
   │   │   │      └──────────────────┐
   │   │   │                         │
   ▼   ▼   ▼                         ▼
┌────────┐ ┌─────────────┐ ┌────────────┐ ┌─────────────┐
│ pool   │ │ lease       │ │ conflict   │ │ fingerprint │
│ alloc  │ │ Manager     │ │ Detector   │ │ Store       │
│ bitmap │ │ (BoltDB)    │ │ (ARP/ICMP) │ │ (BoltDB)    │
└────────┘ └──────┬──────┘ └────────────┘ └─────────────┘
                  │
                  ▼
          ┌───────────────┐
          │  events.Bus   │  buffered channel, fan-out to subscribers
          └──┬──┬─────┬──┬┘
             │  │     │  │
    ┌───────┘   │     │  └──────────┐
    ▼           ▼     ▼             ▼
┌─────────┐ ┌─────┐ ┌───────┐ ┌─────────────┐
│ Dispatch│ │ DDNS| │ SSE   │ │ syslog fwd  │
│ (hooks) │ │ Mgr | │ Hub   │ │ (RFC 5424)  │
└─────────┘ └─────┘ └───────┘ └─────────────┘

         ┌────────────────────┐
         │  dbconfig.Store    │  BoltDB-backed dynamic config
         │  (subnets, hooks,  │  managed via API/web UI
         │   ddns, dns, etc)  │  synced between HA peers
         └────────────────────┘
```

## package layout

everything follows standard Go conventions. `internal/` for private packages, `pkg/` for truly reusable stuff, `cmd/` for the binary entry point

```
cmd/athena-dhcpd/main.go     — entry point, wiring, signal handling
internal/
  anomaly/
    detector.go               — anomaly detection (MAC flapping, lease storms)
  api/
    server.go                 — HTTP server, route registration
    auth.go                   — Bearer token + Basic auth + session cookie middleware
    handlers.go               — lease, subnet, pool, conflict, stats endpoints
    handlers_v2.go            — DB-backed config CRUD (subnets, defaults, hooks, etc)
    handlers_reservations.go  — reservation CRUD + CSV import/export
    handlers_fingerprint.go   — device fingerprint endpoints
    handlers_audit.go         — audit log query + export
    handlers_events.go        — event list, hooks list, test hook
    handlers_ha.go            — HA status + manual failover
    sse.go                    — SSE hub for live event streaming
    spa.go                    — SPA fallback handler (serves embedded React app)
    metrics_middleware.go     — HTTP request metrics
  audit/
    log.go                    — BoltDB-backed audit log
  config/
    config.go                 — TOML parsing, validation, defaults
    write_ha.go               — TOML file writer for HA section
  conflict/
    detector.go               — coordinates ARP + ICMP probing
    arp.go                    — raw socket ARP prober
    icmp.go                   — ICMP echo prober
    table.go                  — conflict table (BoltDB + in-memory)
    cache.go                  — probe result cache (TTL-based)
    gratuitous.go             — gratuitous ARP sender
  dbconfig/
    store.go                  — BoltDB-backed dynamic config store
                                manages all DB-backed config sections
                                merges with TOML bootstrap via BuildConfig()
                                syncs between HA peers
  ddns/
    manager.go                — DDNS lifecycle (subscribe to events, create/remove records)
    rfc2136.go                — RFC 2136 DNS UPDATE client with TSIG
    api_powerdns.go           — PowerDNS HTTP API client
    api_technitium.go         — Technitium HTTP API client
    helpers.go                — FQDN construction, hostname sanitization, reverse IP
  dhcp/                       — the DHCP engine
    handler.go                — DORA message handler + fingerprint extraction
    server.go                 — UDP server loop
    packet.go                 — packet encode/decode
    options.go                — option serialization
    options_registry.go       — option type registry (adding new options = one entry here)
    relay.go                  — Option 82 relay agent info parsing
    ratelimit.go              — per-MAC and global rate limiting
  dnsproxy/
    server.go                 — built-in DNS proxy, caching, filter lists, DoH
  events/
    bus.go                    — buffered event bus with pub/sub
    types.go                  — event types, payloads, env var conversion
    dispatcher.go             — routes events to matching hooks
    script.go                 — script executor (bounded goroutine pool)
    webhook.go                — webhook sender (retries, HMAC, templates)
  fingerprint/
    fingerprint.go            — DHCP fingerprinting, local heuristic classification
    fingerbank.go             — Fingerbank API v2 client for enhanced classification
  ha/
    fsm.go                    — failover state machine (5 states)
    peer.go                   — TCP peer connection, heartbeat, reconnect
    protocol.go               — wire format (length-prefixed JSON messages)
  vip/
    manager.go                — floating VIP group (acquire/release on HA failover, GARP)
  hostname/
    sanitiser.go              — hostname cleanup and deduplication
  lease/
    types.go                  — Lease struct, states
    store.go                  — BoltDB persistence, indexes
    manager.go                — lease lifecycle (offer, ack, renew, release, expire)
    gc.go                     — garbage collector for expired leases
  logging/
    logger.go                 — slog setup helpers
  macvendor/
    lookup.go                 — OUI database for MAC vendor identification
  metrics/
    metrics.go                — all Prometheus metric definitions
  pool/
    allocator.go              — bitmap-based IP allocator (O(1) allocate/release)
    matcher.go                — pool selection based on relay/vendor/user class
  portauto/
    engine.go                 — port automation rule engine
  radius/
    client.go                 — RADIUS client for DHCP authentication
  rogue/
    detector.go               — rogue DHCP server detection
  syslog/
    syslog.go                 — remote syslog forwarder (RFC 5424 over UDP/TCP)
  topology/
    map.go                    — network topology tree from relay agent data
  webui/
    embed.go                  — go:embed for the React SPA dist/
pkg/dhcpv4/
  constants.go               — option codes, message types, HA states, detection methods
  encoding.go                — IP/bytes conversion helpers
web/                          — React + TypeScript + Tailwind frontend source
```

## design principles

### no global mutable state
everything is dependency-injected via structs. the `Handler` gets a lease manager, pool map, conflict detector, event bus, and logger passed to it. no package-level variables holding runtime state (metrics are the exception, Prometheus requires global registration)

### the option registry pattern
adding a new DHCP option doesn't require touching the packet handler, server, or config parser. you add one entry to `optionRegistry` in `options_registry.go` and it just works. the registry defines the option's code, name, type, and length constraints. serialization, deserialization, and validation all flow from the registry

### bitmap pool allocator
IP pools don't do linear scans. each pool is backed by a bitmap — one bit per IP in the range. allocation finds the first zero bit (O(64) worst case per word), release flips a bit. this means allocation scales to /16 pools without slowing down

### event bus decoupling
the DHCP handler doesn't know about hooks, DNS, WebSocket, or anything downstream. it publishes events to the bus and moves on. subscribers (dispatcher, DDNS manager, WebSocket hub) each get their own buffered channel. if a subscriber is slow, its events get dropped independently — never blocks the DHCP hot path

### conflict detection is a pre-offer check
the detector sits between pool allocation and DHCPOFFER. the handler asks the detector "is this IP safe?" and the detector returns immediately if cached/tabled, or probes and returns within the timeout. if probing is unavailable, it returns "clear" with a warning. the DHCP flow never hangs on a missing capability

### lease lookups are O(1) three ways
BoltDB has buckets indexed by IP, MAC, and client-id. the in-memory index mirrors this. you can look up a lease by any of these keys in constant time. the GC runs periodically (default 60s) to clean up expired leases

## concurrency model

- **DHCP server**: single goroutine reads UDP packets, dispatches each to the handler
- **handler**: processes one packet at a time per call (the server can call concurrently if needed)
- **event bus**: publishes are non-blocking (buffered channel). if buffer is full, events are dropped
- **dispatcher**: single goroutine reads from its subscription channel, dispatches to script/webhook workers
- **script runner**: bounded goroutine pool (semaphore pattern, configurable concurrency)
- **webhook sender**: unbounded goroutines (each webhook fires a goroutine), but the HTTP client has a connection pool
- **DDNS manager**: single goroutine reads events, spawns async goroutines for DNS updates
- **HA peer**: multiple goroutines — accept loop, connect loop, heartbeat sender, timeout checker, per-connection handler
- **SSE hub**: single goroutine broadcasts to all connected SSE clients
- **lease GC**: single goroutine on a ticker
- **syslog forwarder**: single goroutine reads from event bus subscription, writes to remote syslog
- **anomaly detector**: single goroutine reads events, maintains sliding window stats
- **rogue detector**: passive monitoring via event bus
- **audit log**: single goroutine writes audit entries to BoltDB
- **fingerprint store**: synchronous on DISCOVER (Record call), async Fingerbank API enrichment in goroutine

all shared state is protected by mutexes. the pool allocator, lease store, conflict table, dbconfig store, and FSM all use `sync.Mutex` or `sync.RWMutex` as appropriate

## error handling

all errors are wrapped with context: `fmt.Errorf("allocating IP for %s: %w", mac, err)`. no panics outside of `main()`. errors in the hot path (packet processing) are logged and the packet is dropped — the client will retry. errors in background tasks (hooks, DNS, HA sync) are logged but never propagate to DHCP processing

## config hot-reload

two kinds of config changes:

**SIGHUP (bootstrap TOML reload):**
1. load and validate the new config file
2. if validation fails, log error, keep old config, done
3. rebuild full config by merging TOML with database state via `dbconfig.BuildConfig()`
4. update handler, lease manager, pools, API server, DNS proxy

**database config changes (via API or web UI):**
1. `dbconfig.Store` persists the change to BoltDB
2. fires `onLocalChange` → triggers HA peer sync for that section
3. fires debounced `onChange` callback in main.go
4. callback rebuilds full config via `BuildConfig()` and updates all components
5. live components (syslog forwarder, Fingerbank client, DNS proxy) are started/stopped/reconfigured as needed

leases, connections, and the event bus are preserved across reloads. database-backed config changes take effect immediately — no restart or SIGHUP needed

## testing approach

- table-driven tests for all encode/decode paths
- mock interfaces for external dependencies (DNS updaters, raw sockets)
- BoltDB tests use temp directories (cleaned up automatically)
- HA tests simulate two nodes via goroutines (no actual TCP, uses in-process connections)
- race detector enabled on all test runs (`-race` flag)
