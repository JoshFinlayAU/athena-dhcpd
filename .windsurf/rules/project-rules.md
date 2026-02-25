---
trigger: always_on
---

# athena-dhcpd — Windsurf / Claude Code Agent Rules

## Project Context
This is `athena-dhcpd`, an RFC-compliant DHCPv4 server written in Go with built-in
high availability via peer lease synchronisation. It stores leases in BoltDB (embedded
key-value store) and uses TOML for configuration. No external databases.

Features: full DHCP option support, relay agent (option 82) handling, **IP conflict
detection via ARP/ICMP probing before every offer**, dynamic DNS integration (BIND,
Knot, PowerDNS, Technitium, Windows DNS, CoreDNS), lease event hooks (scripts +
webhooks), and an embedded web management interface (React SPA compiled into the Go
binary via go:embed).

The full architecture, RFC compliance matrix, conflict detection design, HA protocol,
DDNS integration spec, event hook system, web UI page specs, config format, and phased
implementation plan are in `.claude/project-plan.md`. ALWAYS read that file before
starting work.

## Language & Framework
- Language: Go 1.22+
- Config: TOML (github.com/BurntSushi/toml)
- Storage: BoltDB (go.etcd.io/bbolt)
- Logging: log/slog (stdlib only)
- DNS updates: github.com/miekg/dns (RFC 2136, TSIG)
- ARP probing: github.com/mdlayher/arp OR raw AF_PACKET (evaluate)
- ICMP probing: golang.org/x/net/icmp + golang.org/x/net/ipv4
- Metrics: github.com/prometheus/client_golang
- WebSocket: github.com/gorilla/websocket
- DHCP packets: github.com/insomniacslk/dhcp OR hand-rolled (evaluate first)
- HA wire format: protobuf or msgpack over TCP
- Frontend: React + TypeScript + Tailwind CSS (Vite build, embedded via go:embed)
- No Python. No YAML. No external databases. No third-party Go loggers.

## Code Style
- Follow standard Go conventions: gofmt, govet, Effective Go
- Use `internal/` for all non-exported packages
- Use `pkg/` only for truly reusable, stable interfaces (constants, encoding helpers)
- Wrap ALL errors with context: `fmt.Errorf("allocating IP for %s: %w", mac, err)`
- No panics outside of main()
- No global mutable state — dependency injection via structs
- Use `context.Context` for cancellation and timeouts on all long-running operations
- Structured logging with `slog` — always include: mac, ip, subnet, msg_type, peer fields
- Cite RFC numbers in code comments where relevant logic is implemented
  e.g. `// RFC 2131 §4.4.1 — probe candidate IP before OFFER`

## Architecture Rules
- Config is parsed once at startup and on SIGHUP — never read config files in hot path
- Packet processing must be allocation-efficient — use sync.Pool for buffers
- Lease lookups by IP, MAC, and client-id must ALL be O(1) (BoltDB buckets + in-memory index)
- Pool allocator must NOT do linear scans — use a bitmap or free-list
- Options are handled via a registry pattern:
  - Adding a new option = one registry entry in options_registry.go
  - No changes to packet handler, server, or config parser required
- HA sync is event-driven (lease changes push to peer), not polling
- Failover state machine has explicit states: PARTNER_UP, PARTNER_DOWN, ACTIVE, STANDBY, RECOVERY

## Conflict Detection Rules
- The conflict detector MUST be called before every DHCPOFFER for dynamically allocated IPs
- Reservations (static leases) are exempt from probing by default (configurable)
- Two probe methods: ARP for directly-attached subnets, ICMP ping for relayed/remote subnets
- The server auto-selects ARP vs ICMP based on whether candidate IP is on a local interface subnet
- ARP raw socket (AF_PACKET) and ICMP socket opened ONCE at startup and shared — NOT per-probe
- Probe timeout enforced via context cancellation — never hang waiting for a reply
- Conflict table persisted in BoltDB `conflicts/` bucket AND cached in memory for O(1) lookup
- Conflict table is synced to HA peer alongside lease data (message types 0x09, 0x0A)
- Probe result cache (TTL-based, default 10s) prevents re-probing recently cleared IPs
- If raw socket creation fails (missing CAP_NET_RAW): log LOUD warning, skip probes, proceed
  The server MUST still function without probing — reduced safety is better than not starting
- Gratuitous ARP (optional) sent after DHCPACK on local subnets to update ARP caches
- DHCPDECLINE from clients adds IP to conflict table with detection_method="client_decline"
- IPs exceeding max_conflict_count (default 3) are permanently flagged — manual clear required
- All probe results logged at debug level with timing; conflicts at warn level
- Conflict events (conflict.detected, conflict.decline, conflict.resolved, conflict.permanent)
  fire through the event bus — available to script hooks, webhooks, web UI
- Two probe strategies: "sequential" (one IP at a time) and "parallel" (probe N candidates simultaneously)
- Parallel probing caps worst-case latency at one probe_timeout regardless of how many conflicts
- Probe cache uses sync.Map or equivalent concurrent-safe structure

## Event Hook Rules
- The event bus is a buffered Go channel — NEVER blocks the DHCP hot path
- If the event buffer is full, drop events with a warning log + metric increment
- Script hooks use os/exec — this is the ONLY permitted use of os/exec in the entire project
- Scripts run in a bounded goroutine pool (configurable concurrency, default 4)
- Script timeout is always enforced via context cancellation — kill on timeout
- Hook failures (script or webhook) NEVER propagate to or affect DHCP processing
- Webhooks use an HTTP client pool with configurable timeout and exponential backoff retry
- Pass lease data to scripts via environment variables (ATHENA_* prefix) AND JSON on stdin
- Conflict events include ATHENA_CONFLICT_METHOD and ATHENA_CONFLICT_RESPONDER_MAC env vars
- Webhooks POST JSON with X-Athena-Event header and optional HMAC signature
- Built-in webhook templates for Slack and Teams — template name in config

## Dynamic DNS Rules
- DDNS is a first-class built-in feature — NOT implemented as a script hook
- DDNS manager subscribes to the event bus like any other consumer
- DDNS updates are ALWAYS asynchronous to DHCP responses — never block an ACK for DNS
- Use github.com/miekg/dns for RFC 2136 UPDATE packets and TSIG signing
- Support three update methods: rfc2136 (BIND/Knot/Windows/CoreDNS), powerdns_api, technitium_api
- Forward zone: A records. Reverse zone: PTR records. Both on lease.ack, remove on release/expire
- FQDN construction priority: client option 81 → hostname+domain → MAC fallback → skip
- DHCID records (RFC 4701) for conflict detection when use_dhcid = true
- Failed DNS updates retry with backoff — cleanup on release/expire is best-effort
- TSIG secrets and API keys MUST be redacted in all log output and API responses
- Per-subnet zone overrides supported via [[ddns.zone_override]] config

## Web UI Rules
- Frontend is React + TypeScript + Tailwind, built with Vite
- Static assets compiled to web/dist/ and embedded in Go binary via go:embed
- NO Node.js runtime dependency — everything is baked into the single Go binary
- SPA routing: all non-/api/, non-/metrics paths serve index.html
- Vite dev server proxies /api to running Go binary for development
- Makefile targets: `build-web` (frontend), `build` (depends on build-web), `dev`
- API responses ALWAYS use consistent JSON format: `{"error": "...", "code": "..."}` for errors
- WebSocket endpoint at /api/v1/events/stream for live event streaming with ping/pong keepalive
- Config write-back: atomic (write temp → rename), always create timestamped backup first
- Config backups stored with 0600 permissions
- Two auth modes supported simultaneously: Bearer token (API) + session cookie (web UI)
- Web UI passwords stored as bcrypt hashes only — NEVER plaintext in config
- Role-based access: admin (full) and viewer (read-only)
- Web UI includes dedicated Conflicts page: active conflicts table, clear/exclude actions, history

## File Organisation
```
cmd/athena-dhcpd/main.go    — entry point, signal handling, CLI flags (stdlib flag only)
internal/config/             — TOML parsing, validation, hot-reload, defaults
internal/dhcp/               — packet encode/decode, option engine, server loop, relay handling
internal/lease/              — lease manager, BoltDB store, GC, types
internal/pool/               — IP allocation from ranges, pool matching (opt82, vendor class)
internal/conflict/           — conflict detector, ARP prober, ICMP prober, conflict table, probe cache, gratuitous ARP
internal/ha/                 — peer connection, lease sync, heartbeat, failover state machine
internal/events/             — event bus, hook dispatcher, script runner, webhook sender, templates
internal/ddns/               — DDNS manager, RFC 2136 client, TSIG, PowerDNS API, Technitium API
internal/api/                — HTTP server, router, auth, all endpoint handlers, WebSocket
internal/webui/              — go:embed static assets, SPA fallback handler
internal/logging/            — slog setup helpers
pkg/dhcpv4/                  — constants (message types, option codes), binary encoding helpers
web/                         — React + TypeScript + Tailwind frontend source
configs/example.toml         — annotated example config
```

## Testing Rules
- Table-driven tests for ALL packet encode/decode paths
- Table-driven tests for ALL option serialise/deserialise
- Every lease state transition needs a test
- **ARP probe tests with mock raw socket — test reply (conflict) and timeout (clear) paths**
- **ICMP probe tests with mock — same paths**
- **Conflict table lifecycle tests: detect → hold → resolve, detect → permanent → manual clear**
- **Probe cache tests: TTL expiry, cache hit, invalidation on DECLINE**
- **Full DORA integration test with mocked ARP: clear path + conflict-and-retry path**
- HA sync must have integration tests simulating two nodes via goroutines
- DNS update tests against mock DNS server (miekg/dns test utilities)
- Hook tests: mock script execution (verify env vars + stdin), mock HTTP for webhooks
- FQDN construction tests: all priority paths, edge cases, sanitisation
- API endpoint tests: auth, CRUD, validation, error responses
- Use stdlib `testing` package — testify sparingly
- Test files live alongside code: `*_test.go`
- Minimum 80% coverage on: internal/dhcp, internal/lease, internal/ha, internal/ddns, internal/events, internal/conflict

## Implementation Phases
Work is divided into 5 phases. Complete each fully (including tests) before moving on.

1. **Phase 1 — Core DHCP Engine + Conflict Detection:** Config, packets, options, lease manager
   (BoltDB), DORA, reservations, event bus, ARP probe, ICMP probe, conflict table, probe cache,
   gratuitous ARP, DHCPDECLINE handling. Exit: real clients get IPs, ARP probes fire before
   every offer, conflicted IPs skipped, leases survive restart.

2. **Phase 2 — Options, Relay & Hooks:** Option 82, classless routes (121), pool matching,
   subnet selection, vendor options, BOOTP, rate limiting, ICMP fallback for relayed subnets,
   script hooks, webhook hooks. Exit: works behind relays, hooks fire on events inc. conflicts.

3. **Phase 3 — Dynamic DNS:** RFC 2136 client, TSIG, forward+reverse zones, DHCID conflict
   detection, PowerDNS API, Technitium API, per-subnet zone overrides.
   Exit: leases auto-register in DNS, cleanup on release/expire.

4. **Phase 4 — High Availability:** Sync protocol, heartbeat, failover state machine,
   bulk sync (leases + conflict table), conflict resolution, TLS, HA events.
   Exit: clean failover, no duplicate leases, conflict table synced.

5. **Phase 5 — Web UI, API & Hardening:** HTTP API (all endpoints inc. conflicts), WebSocket,
   React SPA (dashboard, leases, reservations, config editor, events, conflicts page, HA status),
   go:embed, auth, Prometheus, SIGHUP reload, graceful shutdown, systemd (CAP_NET_RAW), Docker.
   Exit: production-deployable single binary with full web management.

## What NOT To Do
- Do NOT use YAML for anything
- Do NOT write Python code
- Do NOT add external database dependencies (no Postgres, MySQL, Redis, etcd, SQLite)
- Do NOT use third-party Go logging (no zap, logrus, zerolog) — slog only
- Do NOT use cobra or viper — stdlib `flag` package only
- Do NOT implement DHCPv6 — this project is DHCPv4 only
- Do NOT use os/exec anywhere EXCEPT internal/events/script_runner.go for hook scripts
- Do NOT store config in the lease database
- Do NOT skip writing tests — every phase must include tests before completion
- Do NOT block DHCP packet processing on hooks, DDNS, or any async operation
- Do NOT block DHCPOFFER on slow conflict probes — enforce timeout, fall through on error
- Do NOT open raw sockets per-probe — open once at startup, share across probes
- Do NOT store plaintext passwords in config — bcrypt hashes only
- Do NOT log TSIG secrets, API keys, or auth tokens — always redact sensitive fields
- Do NOT require Node.js at runtime — frontend is compiled and embedded at build time
- Do NOT use a separate web server for the UI — it's served from the Go binary
- Do NOT skip probes silently — if CAP_NET_RAW is missing, log a LOUD startup warning

## Security
- Validate ALL incoming DHCP packet fields — drop malformed with warning, never crash
- Rate limit per-MAC to prevent DHCP starvation (RFC 5765)
- HA peer communication should use TLS in production
- API requires auth token if bound to non-loopback
- Web UI passwords: bcrypt only, never plaintext
- TSIG secrets and API keys: redacted in all logs and API responses
- Config backups: 0600 permissions, not world-readable
- Script hooks: run with timeout, capture stderr, never trust exit environment
- WebSocket: authenticate before upgrade, ping/pong keepalive
- **CAP_NET_RAW required for ARP/ICMP probes** — document in README, systemd unit, Dockerfile

## Performance Targets
- 1000+ DORA cycles/second on a single core (with probe cache warm)
- Sub-millisecond lease lookup by IP, MAC, or client-id
- Lease DB should handle 100k+ active leases without degradation
- HA sync latency < 10ms per lease update on LAN
- Event bus throughput: 10k+ events/sec without DHCP backpressure
- Web UI API response time: < 50ms for lease queries, < 200ms for filtered searches
- ARP/ICMP probe: adds max `probe_timeout` (default 500ms) per candidate IP
- Parallel probe strategy: worst-case one timeout regardless of conflict count
- Probe cache eliminates repeated probes within TTL window
