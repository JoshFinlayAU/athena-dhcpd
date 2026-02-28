# athena-dhcpd

A DHCPv4 server that actually works. Written in Go because life is too short for ISC DHCP config files

ships as a single binary. no java, no python, no "install these 47 dependencies first". just a binary and a config file. you could probably run it on a toaster

## why does this exist

because every time I looked at ISC dhcpd's config syntax I lost the will to live. and Kea needs a PhD in YAML to configure. I wanted something that:

- hands out IPs (shocking, I know)
- doesnt hand out IPs that something else is already using (you'd think this would be standard)
- tells your DNS server about it automatically
- keeps working when a server dies
- lets you see whats happening without grepping through log files like its 2003

## features

the whole kitchen sink, basically

### core DHCP stuff
- Full DORA cycle (Discover, Offer, Request, Ack) per RFC 2131
- BOOTP support because apparently people still use that
- Lease management with BoltDB - embedded, no external database nonsense
- Automatic lease expiry and garbage collection
- Bitmap-based IP pool allocator. no linear scans, we're not animals
- Static reservations by MAC or client identifier
- Rate limiting so one misbehaving client cant DOS your DHCP server
- Relay agent support (option 82) with circuit ID, remote ID, and link selection
- Pool matching on circuit ID, remote ID, vendor class (option 60), user class (option 77) with glob patterns

### conflict detection (the cool part)
Before handing out an IP, athena actually checks if something else is using it. revolutionary concept

- ARP probing on local subnets (raw sockets, needs CAP_NET_RAW)
- ICMP ping for relayed/remote subnets
- auto-selects ARP vs ICMP based on whether the IP is on a local interface
- probe result caching so we're not ARPing the same IP 50 times a second
- sequential or parallel probe strategies
- conflict table persisted in BoltDB, survives restarts
- IPs that keep conflicting get permanently flagged (configurable threshold)
- DHCPDECLINE handling - if a client says "this IP is taken" we believe them
- gratuitous ARP after ACK to update switch tables
- if raw socket creation fails it just logs a warning and keeps going. degraded but functional is better than dead

### dynamic DNS
Built in, not bolted on as a script hook

- RFC 2136 dynamic updates with TSIG signing
- Forward (A) and reverse (PTR) records
- DHCID records for conflict detection (RFC 4701)
- FQDN construction: client option 81 → hostname + domain → MAC fallback
- Supports BIND, Knot, Windows DNS, CoreDNS via RFC 2136
- PowerDNS API support
- Technitium API support
- Per-subnet zone overrides
- Updates are async - never blocks a DHCP response waiting for DNS
- cleanup on lease release/expire (best effort, DNS is like that sometimes)

### high availability
Active-standby failover with lease synchronization

- TCP peer connection with heartbeat
- Event-driven lease sync (not polling)
- Bulk sync on reconnect
- Conflict table synced alongside leases
- Explicit state machine: PARTNER_UP, PARTNER_DOWN, ACTIVE, STANDBY, RECOVERY
- Optional TLS for peer communication
- manual failover trigger via API

### device fingerprinting
know what's on your network without an agent on every device

- extracts DHCP fingerprint data from every DISCOVER (option 55 parameter list, option 60 vendor class, hostname)
- local heuristic classification identifies Windows, macOS, iOS, Android, Linux, printers, network gear, cameras, embedded devices
- optional Fingerbank API integration for much more accurate classification. free API key from api.fingerbank.org
- persistent storage in BoltDB — survives restarts
- web UI page with device table, type/OS stats, search/filter, and inline API key setup

### rogue DHCP server detection
someone plugged in a consumer router again

- passively monitors for DHCP offers from servers that aren't us
- tracks rogue server IP, MAC, offered IPs, and client MACs
- fires events through the event bus for alerting
- web UI page with rogue server list and acknowledge/remove actions
- on-demand active scan via API

### anomaly detection
automatically spots weird DHCP behaviour

- detects MAC flapping, lease storms, unusual request patterns
- weather-style status indicator in the web UI (calm, advisory, watch, warning, critical)
- fires events for hook integration

### audit log
who did what and when

- logs all API write operations and DHCP state changes to BoltDB
- query by time range, event type, user, IP, MAC
- export as CSV
- stats endpoint for audit activity breakdown

### remote syslog forwarding
forward events to your existing log infrastructure

- RFC 5424 syslog messages over UDP or TCP
- configurable facility, tag, and protocol
- auto-reconnect on connection failure
- all DHCP events, conflicts, HA failovers, and rogue detections forwarded

### event hooks
Things happen, you probably want to know about them

- Buffered event bus that never blocks the DHCP hot path
- Script hooks via os/exec with configurable concurrency and timeouts
- Webhook hooks with retries, exponential backoff, and HMAC signing
- Built-in Slack and Teams webhook templates
- Events: lease.discover, lease.offer, lease.ack, lease.release, lease.expire, lease.decline, conflict.detected, conflict.resolved, conflict.permanent, ha.failover, etc
- Lease data passed to scripts via ATHENA_* environment variables AND JSON on stdin
- Hook failures never affect DHCP processing. your slack is down? not our problem

### built-in DNS proxy
why run a separate DNS server when your DHCP server already knows every hostname on the network

- full DNS proxy with local zone, caching, and upstream forwarding
- auto-registers A and PTR records from DHCP leases. clients can resolve each other by hostname immediately
- DNS-over-HTTPS support (RFC 8484) as both server and client
- zone overrides for split-horizon DNS — route `corp.internal` to one nameserver, everything else upstream
- static records for stuff that isnt a DHCP client
- response caching with configurable size and TTL
- UDP, TCP, and DoH all go through the same query pipeline
- domain filter lists — basically a built-in pihole
  - blocklists and allowlists with automatic refresh
  - supports hosts file, plain domain, and adblock filter formats
  - block actions: NXDOMAIN, 0.0.0.0, or REFUSED
  - allowlists always win over blocklists
  - subdomain matching (block `ads.example.com` also blocks `sub.ads.example.com`)
  - test any domain against your lists via API or web UI
- if you dont need it just dont enable it. zero overhead when disabled

### hostname sanitisation
clients send garbage hostnames. we clean them up

- strips invalid characters, enforces length limits
- deduplication — if two clients claim the same hostname, the second one gets a suffix
- configurable allowed character set and max length
- optional lookup function to check for conflicts before accepting

### MAC vendor lookup
know who made the thing

- built-in OUI database for MAC address vendor identification
- API endpoint for on-demand lookups
- enriches lease data in the web UI

### topology mapping
visualize your network

- builds a tree from relay agent data (option 82 circuit ID, remote ID)
- tracks which switch port each client is connected to
- custom labels for switches and ports
- web UI topology page with tree view and stats

### RADIUS integration
authenticate DHCP clients before handing out IPs

- per-subnet RADIUS server configuration
- Access-Request with MAC, circuit ID, and NAS-IP
- test connectivity via API

### port automation
trigger switch port changes based on DHCP events

- rule-based automation (e.g. auto-enable port on lease, disable on expiry)
- configurable via API
- test rules before deploying

### web UI
React + TypeScript + Tailwind. dark mode because we have taste

- **Setup wizard** — first boot walks you through deployment mode, HA, subnets, pools, reservations, conflict detection, and DNS proxy. import reservations from CSV, JSON, ISC dhcpd, dnsmasq, Kea, or MikroTik
- Dashboard with real-time stats, pool utilization, live event feed
- Lease browser with search, filtering, pagination, and live updates via SSE
- Full configuration editor — subnets, pools, reservations, defaults, conflict detection, HA, hooks, DDNS, DNS proxy, syslog, fingerprinting, hostname sanitisation. all stored in the database and editable live
- Reservation management with multi-format import (CSV, JSON, ISC dhcpd, dnsmasq, Kea, MikroTik)
- Conflict viewer with clear and permanent-exclude actions
- Device fingerprints page with type/OS breakdown and Fingerbank API key setup
- Rogue DHCP server detection page with acknowledge/remove actions
- Live event stream. watch packets fly by in real time
- Audit log viewer with search, filtering, and CSV export
- HA cluster status and manual failover controls
- DNS proxy dashboard with query log, cache stats, filter list management
- Network topology tree view
- Role-based auth: admin gets write access, viewer gets read-only
- Bearer token auth for API, session cookies for the web UI
- Passwords stored as bcrypt hashes. we're not storing passwords in plaintext in 2025

the whole frontend compiles into the Go binary via go:embed. zero runtime dependencies. no node.js on your DHCP server

### REST API
Everything the web UI does, the API can do too. all endpoints are under `/api/v2/`

```
GET    /api/v2/health
GET    /api/v2/stats
POST   /api/v2/auth/login              session login
POST   /api/v2/auth/logout             session logout
GET    /api/v2/auth/me                 current user info
GET    /api/v2/leases                  list/search/filter/paginate
GET    /api/v2/leases/export           CSV export
GET    /api/v2/leases/{ip}
DELETE /api/v2/leases/{ip}
GET    /api/v2/reservations            flat view across all subnets
POST   /api/v2/reservations
PUT    /api/v2/reservations/{id}
DELETE /api/v2/reservations/{id}
POST   /api/v2/reservations/import
GET    /api/v2/reservations/export
GET    /api/v2/subnets
GET    /api/v2/pools
GET    /api/v2/conflicts
GET    /api/v2/conflicts/stats
GET    /api/v2/conflicts/history
DELETE /api/v2/conflicts/{ip}
POST   /api/v2/conflicts/{ip}/exclude
GET    /api/v2/config/subnets          DB-backed CRUD
POST   /api/v2/config/subnets
PUT    /api/v2/config/subnets/{network}
DELETE /api/v2/config/subnets/{network}
GET    /api/v2/config/defaults
PUT    /api/v2/config/defaults
GET    /api/v2/config/conflict
PUT    /api/v2/config/conflict
GET    /api/v2/config/ha               reads from TOML
PUT    /api/v2/config/ha               writes to TOML
GET    /api/v2/config/hooks
PUT    /api/v2/config/hooks
GET    /api/v2/config/ddns
PUT    /api/v2/config/ddns
GET    /api/v2/config/dns
PUT    /api/v2/config/dns
GET    /api/v2/config/syslog
PUT    /api/v2/config/syslog
GET    /api/v2/config/fingerprint
PUT    /api/v2/config/fingerprint
GET    /api/v2/config/hostname-sanitisation
PUT    /api/v2/config/hostname-sanitisation
POST   /api/v2/config/import           TOML import
GET    /api/v2/config/raw              running config as TOML
POST   /api/v2/config/validate
GET    /api/v2/ha/status
POST   /api/v2/ha/failover
GET    /api/v2/dns/stats
GET    /api/v2/dns/records
POST   /api/v2/dns/cache/flush
GET    /api/v2/dns/lists
POST   /api/v2/dns/lists/refresh
POST   /api/v2/dns/lists/test
GET    /api/v2/dns/querylog
GET    /api/v2/dns/querylog/stream     SSE
GET    /api/v2/events                  recent events
GET    /api/v2/events/stream           SSE live event stream
GET    /api/v2/hooks
POST   /api/v2/hooks/test
GET    /api/v2/audit                   query audit log
GET    /api/v2/audit/export            CSV export
GET    /api/v2/audit/stats
GET    /api/v2/fingerprints            device fingerprints
GET    /api/v2/fingerprints/stats
GET    /api/v2/fingerprints/{mac}
GET    /api/v2/rogue                   rogue DHCP servers
GET    /api/v2/rogue/stats
POST   /api/v2/rogue/scan
GET    /api/v2/topology                network topology tree
GET    /api/v2/topology/stats
GET    /api/v2/anomaly/weather         anomaly status
GET    /api/v2/macvendor/{mac}         MAC vendor lookup
GET    /api/v2/radius                  RADIUS config
GET    /api/v2/portauto/rules          port automation rules
GET    /api/v2/setup/status            setup wizard
GET    /api/v2/backup                  full backup export
POST   /api/v2/backup/restore
GET    /metrics                        Prometheus
```

theres more endpoints than that but you get the idea

### monitoring
- Prometheus metrics for everything. leases, pools, conflicts, DHCP message counts, API latency, DNS updates, HA state, the works
- SSE event streaming for live dashboards
- structured JSON logging via slog

## build dependencies

you need Go 1.22+ and Node.js 20+ (only for building the frontend, not at runtime)

on debian 12 or 13 theres a script that handles everything:
```bash
sudo ./scripts/install-build-deps.sh
```

it installs Go from golang.org (debian 12's packaged Go is too old), Node.js from NodeSource (debian 12's Node 18 is too old for Vite), and the usual build tools (`build-essential`, `git`, `dpkg-dev`, etc). on debian 13 the distro packages are fine so it just uses those

on other distros, you need:
- **Go 1.22+** — https://go.dev/dl/
- **Node.js 20.19+** — https://nodejs.org/ (only for building, not at runtime)
- **make**, **git**
- **dpkg-dev**, **apt-utils** (only if building .deb packages)
- **libcap2-bin** (for `setcap` to set capabilities on the binary)

## building

```bash
# everything including the web UI
make build

# just the Go binary (no web UI)
go build -o athena-dhcpd ./cmd/athena-dhcpd

# build a .deb package
make build-deb

# development mode (web UI with hot reload)
cd web && npm run dev    # frontend on :5173
make dev                 # Go backend
```

## installing

### from source (make install)

builds everything and installs it to the system. run as root

```bash
sudo make install
```

this does:
- builds the binary with the web UI embedded
- installs to `/usr/local/bin/athena-dhcpd`
- copies example config to `/etc/athena-dhcpd/config.toml` (wont overwrite existing)
- creates `/var/lib/athena-dhcpd` for lease data
- installs the systemd service file
- **sets `CAP_NET_RAW` and `CAP_NET_BIND_SERVICE`** on the binary so it can do ARP probing and bind port 53/67 without running as root

then just:
```bash
# edit your config
sudo vim /etc/athena-dhcpd/config.toml

# start it
sudo systemctl enable --now athena-dhcpd
```

### from .deb package

```bash
make build-deb
sudo dpkg -i build/athena-dhcpd_*.deb
```

the deb package does the same stuff as `make install` plus:
- creates a dedicated `athena-dhcpd` system user/group
- sets file permissions (config is 0640, data dir is 0750)
- **sets `CAP_NET_RAW` and `CAP_NET_BIND_SERVICE`** via postinst
- enables the systemd service (but doesnt start it on first install so you can edit the config first)

### manual

if you want to do it yourself:
```bash
# build
make build

# copy binary
sudo install -m 0755 build/athena-dhcpd /usr/local/bin/

# set capabilities (required for ARP conflict detection + binding port 53/67)
sudo setcap 'cap_net_raw,cap_net_bind_service+ep' /usr/local/bin/athena-dhcpd

# copy config
sudo mkdir -p /etc/athena-dhcpd
sudo cp configs/example.toml /etc/athena-dhcpd/config.toml

# copy systemd service
sudo cp deploy/athena-dhcpd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now athena-dhcpd
```

### docker

```bash
docker build -t athena-dhcpd .
docker run --cap-add=NET_RAW --cap-add=NET_BIND_SERVICE \
  --network=host \
  -v /etc/athena-dhcpd:/etc/athena-dhcpd \
  -v /var/lib/athena-dhcpd:/var/lib/athena-dhcpd \
  athena-dhcpd
```

you need `--network=host` because DHCP uses broadcast. thats just how DHCP works, don't @ me

`--cap-add=NET_RAW` is needed for ARP conflict detection. `--cap-add=NET_BIND_SERVICE` for binding port 53 (DNS) and 67 (DHCP). without `NET_RAW` it still works but conflict detection is disabled and you get a loud warning in the logs

## capabilities

athena-dhcpd needs two Linux capabilities to do its thing properly:

| Capability | Why |
|------------|-----|
| `CAP_NET_RAW` | ARP probing for IP conflict detection. without this, conflict detection is disabled (server still works, just less safe) |
| `CAP_NET_BIND_SERVICE` | Binding to privileged ports: UDP/TCP 67 (DHCP) and 53 (DNS proxy) |

these are set automatically by:
- `make install` (via `setcap`)
- the .deb package (via `postinst`)
- the systemd service file (via `AmbientCapabilities`)
- docker (via `--cap-add`)

if you're running as root you dont need any of this but running a network service as root in 2025 is a choice

## configuration

athena-dhcpd uses a two-layer config model:

1. **Bootstrap TOML** (`/etc/athena-dhcpd/config.toml`) — just `[server]`, `[api]`, and optionally `[ha]`. the API and web UI always start — no need to enable them. see `configs/example.toml`
2. **Database** (BoltDB) — everything else: subnets, pools, reservations, defaults, conflict detection, hooks, DDNS, DNS proxy, syslog, fingerprinting, hostname sanitisation. managed through the web UI or API, synced between HA peers automatically

on first startup with no config in the database, the setup wizard walks you through the initial config. you can also import a full TOML config file from the web UI if you're migrating

HA config (`[ha]`) stays in TOML because its node-identity — each node needs its own role and peer address. the web UI can still edit it (it writes directly to the TOML file)

hot-reload via SIGHUP:
```bash
kill -HUP $(cat /run/athena-dhcpd/athena-dhcpd.pid)
```

reloads bootstrap config (server, api, ha) without dropping leases or connections. database-backed config changes take effect immediately through the API — no restart needed

## config validation

the config parser actually validates things, unlike some DHCP servers I could name

- overlapping subnet detection
- overlapping pool range detection
- pool range ordering (start must be before end, yes someone will get this wrong)
- required field validation
- sane defaults for everything

## project structure

```
cmd/athena-dhcpd/       entry point
internal/
  anomaly/              anomaly detection (MAC flapping, lease storms)
  api/                  REST API + SSE + auth
  audit/                audit log (BoltDB-backed)
  config/               TOML parsing + validation
  conflict/             ARP/ICMP probing + conflict table
  dbconfig/             BoltDB-backed dynamic config store
  ddns/                 dynamic DNS (RFC 2136, PowerDNS, Technitium)
  dhcp/                 packet handling, options, server loop
  dnsproxy/             built-in DNS proxy + filter lists
  events/               event bus, script hooks, webhooks
  fingerprint/          DHCP fingerprinting + Fingerbank API client
  ha/                   peer sync, heartbeat, failover FSM
  hostname/             hostname sanitisation + deduplication
  lease/                BoltDB store + manager + GC
  logging/              slog setup
  macvendor/            OUI database for MAC vendor lookup
  metrics/              Prometheus metrics
  pool/                 bitmap allocator + pool matching
  portauto/             port automation engine
  radius/               RADIUS client for DHCP auth
  rogue/                rogue DHCP server detector
  syslog/               remote syslog forwarder (RFC 5424)
  topology/             network topology map from relay data
  webui/                embedded frontend (go:embed)
pkg/dhcpv4/             constants + encoding helpers
web/                    React frontend source
```

## testing

```bash
make test           # with -race
make test-coverage  # html coverage report
```

table-driven tests for packet encode/decode, option serialization, every lease state transition, ARP/ICMP probe paths, conflict table lifecycle, DORA integration, HA sync, DNS updates, API endpoints, and pool matching. its a lot of tests

## license

MIT. do whatever you want with it
