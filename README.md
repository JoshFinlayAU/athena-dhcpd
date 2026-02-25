# athena-dhcpd

A DHCPv4 server that actually works. Written in Go because life is too short for ISC DHCP config files

ships as a single binary. no java, no python, no "install these 47 dependencies first". just a binary and a TOML config file. you could probably run it on a toaster

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

### web UI
React + TypeScript + Tailwind. dark mode because we have taste

- Dashboard with real-time stats, pool utilization, live event feed
- Lease browser with search, filtering, pagination, and live updates via WebSocket
- Reservation management - add, edit, delete
- Conflict viewer with clear and permanent-exclude actions
- Live event stream. watch packets fly by in real time
- HA cluster status and manual failover controls  
- Config editor with validation (its TOML, you can read it)
- Role-based auth: admin gets write access, viewer gets read-only
- Bearer token auth for API, session cookies for the web UI
- Passwords stored as bcrypt hashes. we're not storing passwords in plaintext in 2025

the whole frontend compiles into the Go binary via go:embed. zero runtime dependencies. no node.js on your DHCP server

### REST API
Everything the web UI does, the API can do too

```
GET    /api/v1/health
GET    /api/v1/leases
GET    /api/v1/leases/{ip}
DELETE /api/v1/leases/{ip}
GET    /api/v1/reservations
POST   /api/v1/reservations
GET    /api/v1/conflicts
GET    /api/v1/conflicts/stats
DELETE /api/v1/conflicts/{ip}
POST   /api/v1/conflicts/{ip}/exclude
GET    /api/v1/config/raw
PUT    /api/v1/config
POST   /api/v1/config/validate
GET    /api/v1/ha/status
POST   /api/v1/ha/failover
GET    /api/v1/dns/stats
GET    /api/v1/dns/records
POST   /api/v1/dns/cache/flush
GET    /api/v1/dns/lists
POST   /api/v1/dns/lists/refresh
POST   /api/v1/dns/lists/test
GET    /api/v1/events/stream          (WebSocket)
GET    /metrics                        (Prometheus)
```

theres more endpoints than that but you get the idea

### monitoring
- Prometheus metrics for everything. leases, pools, conflicts, DHCP message counts, API latency, DNS updates, HA state, the works
- WebSocket event streaming for live dashboards
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

TOML. because its readable by humans and also by computers. see `configs/example.toml` for a fully annotated example

hot-reload via SIGHUP:
```bash
kill -HUP $(cat /run/athena-dhcpd/athena-dhcpd.pid)
```

reloads config, pools, and rate limiter settings without dropping any leases or connections. if the new config is broken it just keeps the old one and logs an error like a reasonable program

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
  api/                  REST API + WebSocket + auth
  config/               TOML parsing + validation  
  conflict/             ARP/ICMP probing + conflict table
  ddns/                 dynamic DNS (RFC 2136, PowerDNS, Technitium)
  dnsproxy/             built-in DNS proxy + filter lists
  dhcp/                 packet handling, options, server loop
  events/               event bus, script hooks, webhooks
  ha/                   peer sync, heartbeat, failover FSM
  lease/                BoltDB store + manager + GC
  logging/              slog setup
  metrics/              Prometheus metrics
  pool/                 bitmap allocator + pool matching
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
