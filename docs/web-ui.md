# Web UI

dark mode React SPA baked into the Go binary. no Node.js runtime, no separate web server, no npm install on your DHCP server. just enable it in the config and go

## enabling

```toml
[api]
listen = "127.0.0.1:8080"
```

the API server and web UI always start. just set the listen address and open it in a browser. thats it

## pages

### Dashboard
the landing page. shows at a glance:

- **stat cards** — total leases, active conflicts, pools, uptime
- **recent events** — live feed from the WebSocket, updates in real time without refreshing
- **conflict summary** — breakdown by subnet and detection method

### Leases
searchable, filterable, paginated table of all leases

- search by IP, MAC, or hostname
- filter by state (active, offered, expired)
- auto-refreshes when lease events come in via WebSocket — no polling, no manual refresh
- click delete to force-release a lease (admin only)
- export to CSV

### Reservations
CRUD interface for static IP reservations

- add new reservations with MAC, IP, hostname, subnet selection
- edit existing reservations inline
- delete with confirmation
- all changes go through the API, which updates the in-memory config

### Conflicts
active IP conflicts table

- shows conflicted IP, detection method, responder MAC, subnet, probe count, when it was detected
- **clear** button — removes the conflict, makes the IP available again
- **exclude** button — permanently flags the IP so it never gets allocated
- auto-updates when conflict events arrive

### Events
live event stream

- events flow in via WebSocket in real time
- color-coded by event type (lease events are blue, conflicts are red/orange, HA is purple)
- filter by event type
- pause/resume the stream
- clear the display
- each event expandable to see full JSON payload

### HA Status
high availability cluster overview

- current role (primary/secondary)
- current state (ACTIVE, STANDBY, PARTNER_UP, etc)
- peer connection status
- last heartbeat timestamp
- **failover button** — triggers manual failover (admin only, be careful with this)

if HA is disabled, shows a message saying so

### Device Fingerprints
DHCP fingerprinting and device classification

- device table with MAC, vendor class, hostname, device type, OS, confidence, source
- stats cards showing breakdown by device type
- search and filter by any field
- if Fingerbank API key is not configured, shows an alert banner with inline key setup
- supports both local heuristic classification and Fingerbank API enrichment

### Rogue DHCP Servers
detected rogue DHCP servers on your network

- table of detected rogue servers with IP, MAC, offered IPs, client MACs, first/last seen
- **acknowledge** button — mark a server as known/expected
- **remove** button — clear the entry
- **scan** button — trigger an active scan
- stats summary

### Audit Log
who did what and when

- searchable, filterable audit trail of all API writes and DHCP state changes
- filter by time range, event type, user, IP, MAC
- export to CSV
- stats breakdown by event type

### Topology
network topology tree built from relay agent data

- tree view showing switches, ports, and connected clients
- custom labels for topology nodes
- stats summary

### Configuration
DB-backed config editor with per-section pages

- subnets, pools, reservations — full CRUD
- defaults, conflict detection, hooks, DDNS, DNS proxy, syslog, fingerprinting, hostname sanitisation, HA — each editable via forms
- all changes go through the API and take effect immediately
- TOML import for migration from other DHCP servers
- raw config view

### Setup Wizard
first-boot guided setup

- walks through deployment mode, HA, subnets, pools, reservations, conflict detection, DNS proxy
- import reservations from CSV, JSON, ISC dhcpd, dnsmasq, Kea, or MikroTik
- only shown when no config exists in the database

## live updates

the web UI connects to `/api/v2/events/stream` via SSE (Server-Sent Events). this means:

- lease changes appear on the Leases page instantly
- conflicts pop up on the Conflicts page as they're detected
- the Dashboard event feed updates in real time
- a green pulsing dot in the sidebar shows connection status

if the SSE connection drops (server restart, network blip), it automatically reconnects. the dot turns grey while disconnected so you know

## authentication

the web UI uses session cookie auth:

- login page with username/password form
- `POST /api/v2/auth/login` sets an `athena_session` cookie
- sessions expire after 24h by default (configurable via `api.session.expiry`)
- if `api.auth.users` has entries in config, those are available. users can also be created via the API
- Bearer token auth still works for API-only access

roles work the same: admin sees all buttons, viewer sees read-only views with action buttons hidden

## tech stack

- React 19 + TypeScript
- Tailwind CSS v4 (dark theme)
- React Router v6 for SPA routing
- Lucide React for icons
- Inter font for UI, JetBrains Mono for IPs/MACs/code
- Vite for building

the whole thing compiles to a few hundred KB of JS + CSS, which gets embedded in the Go binary via `go:embed`. the SPA fallback handler serves `index.html` for all non-API paths so client-side routing works

## development

for working on the frontend with hot reload:

```bash
# terminal 1: Go backend
make dev

# terminal 2: Vite dev server with API proxy
cd web && npm run dev
```

Vite proxies `/api/*` and `/metrics` to `http://localhost:8080` (the Go backend) and `/ws` connections get proxied as WebSocket. hot module replacement works so you see changes instantly

## building for production

```bash
make build
```

this runs `npm ci && npm run build` in the `web/` directory, copies `web/dist/` to `internal/webui/dist/`, then builds the Go binary with `go:embed`. the result is a single binary with the entire frontend baked in

## customization

the UI uses CSS custom properties for theming, defined in `web/src/index.css`. if you want different colors, change the `@theme` block. the dark mode is the only mode because light mode DHCP management sounds like a punishment
