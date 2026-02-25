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
┌─────────────────┐
│  dhcp.Handler   │  DORA cycle: Discover→Offer→Request→Ack
│  (handler.go)   │  + Decline, Release, Inform
└──┬───┬───┬──────┘
   │   │   │
   │   │   └──────────────────────┐
   │   │                          │
   ▼   ▼                          ▼
┌──────────┐  ┌───────────────┐  ┌──────────────┐
│ pool     │  │ lease.Manager │  │ conflict     │
│ allocator│  │ (BoltDB)      │  │ Detector     │
│ (bitmap) │  │               │  │ (ARP/ICMP)   │
└──────────┘  └───────┬───────┘  └──────────────┘
                      │
                      ▼
              ┌───────────────┐
              │  events.Bus   │  buffered channel, fan-out to subscribers
              └──┬───┬───┬───┘
                 │   │   │
        ┌────────┘   │   └────────┐
        ▼            ▼            ▼
┌─────────────┐ ┌─────────┐ ┌──────────┐
│ Dispatcher  │ │ DDNS    │ │ WS Hub   │
│ (hooks)     │ │ Manager │ │ (live    │
│             │ │         │ │  events) │
└─────────────┘ └─────────┘ └──────────┘
```

## package layout

everything follows standard Go conventions. `internal/` for private packages, `pkg/` for truly reusable stuff, `cmd/` for the binary entry point

```
cmd/athena-dhcpd/main.go     — entry point, wiring, signal handling
internal/
  config/                     — TOML parsing, validation, defaults, hot-reload
  dhcp/                       — the DHCP engine
    handler.go                — DORA message handler
    server.go                 — UDP server loop
    packet.go                 — packet encode/decode
    options.go                — option serialization
    options_registry.go       — option type registry (adding new options = one entry here)
    relay.go                  — Option 82 relay agent info parsing
    ratelimit.go              — per-MAC and global rate limiting
  lease/
    types.go                  — Lease struct, states
    store.go                  — BoltDB persistence, indexes
    manager.go                — lease lifecycle (offer, ack, renew, release, expire)
    gc.go                     — garbage collector for expired leases
  pool/
    allocator.go              — bitmap-based IP allocator (O(1) allocate/release)
    matcher.go                — pool selection based on relay/vendor/user class
  conflict/
    detector.go               — coordinates ARP + ICMP probing
    arp.go                    — raw socket ARP prober
    icmp.go                   — ICMP echo prober
    table.go                  — conflict table (BoltDB + in-memory)
    cache.go                  — probe result cache (TTL-based)
    gratuitous.go             — gratuitous ARP sender
  events/
    bus.go                    — buffered event bus with pub/sub
    types.go                  — event types, payloads, env var conversion
    dispatcher.go             — routes events to matching hooks
    script.go                 — script executor (bounded goroutine pool)
    webhook.go                — webhook sender (retries, HMAC, templates)
  ddns/
    manager.go                — DDNS lifecycle (subscribe to events, create/remove records)
    rfc2136.go                — RFC 2136 DNS UPDATE client with TSIG
    api_powerdns.go           — PowerDNS HTTP API client
    api_technitium.go         — Technitium HTTP API client
    helpers.go                — FQDN construction, hostname sanitization, reverse IP
  ha/
    fsm.go                    — failover state machine (5 states)
    peer.go                   — TCP peer connection, heartbeat, reconnect
    protocol.go               — wire format (length-prefixed JSON messages)
  api/
    server.go                 — HTTP server, route registration
    auth.go                   — Bearer token + Basic auth middleware
    handlers.go               — lease, subnet, pool, conflict, stats endpoints
    handlers_reservations.go  — reservation CRUD + CSV import/export
    handlers_config.go        — config read/write/validate/backup
    handlers_events.go        — event list, hooks list, test hook
    handlers_ha.go            — HA status + manual failover
    websocket.go              — WebSocket hub for live event streaming
    spa.go                    — SPA fallback handler (serves embedded React app)
    metrics_middleware.go     — HTTP request metrics
  webui/
    embed.go                  — go:embed for the React SPA dist/
  logging/
    logger.go                 — slog setup helpers
  metrics/
    metrics.go                — all Prometheus metric definitions
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
- **WebSocket hub**: single goroutine broadcasts to all connected clients
- **lease GC**: single goroutine on a ticker

all shared state is protected by mutexes. the pool allocator, lease store, conflict table, and FSM all use `sync.Mutex` or `sync.RWMutex` as appropriate

## error handling

all errors are wrapped with context: `fmt.Errorf("allocating IP for %s: %w", mac, err)`. no panics outside of `main()`. errors in the hot path (packet processing) are logged and the packet is dropped — the client will retry. errors in background tasks (hooks, DNS, HA sync) are logged but never propagate to DHCP processing

## config hot-reload

on SIGHUP:
1. load and validate the new config file
2. if validation fails, log error, keep old config, done
3. update the lease manager's config reference
4. update the handler's config reference
5. reinitialize pools from the new config
6. update the handler's pool map

leases, connections, and the event bus are preserved across reloads. the only thing that requires a restart is changing the network interface or bind address

## testing approach

- table-driven tests for all encode/decode paths
- mock interfaces for external dependencies (DNS updaters, raw sockets)
- BoltDB tests use temp directories (cleaned up automatically)
- HA tests simulate two nodes via goroutines (no actual TCP, uses in-process connections)
- race detector enabled on all test runs (`-race` flag)
