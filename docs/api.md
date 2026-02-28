# API Reference

the REST API runs on whatever you set `api.listen` to (default `0.0.0.0:8067`). all responses are JSON unless noted otherwise. all endpoints are under `/api/v2/`

## Authentication

four ways to authenticate:

1. **Session cookie** — login via `POST /api/v2/auth/login` with username/password, get a session cookie. this is what the web UI uses
2. **Bearer token** — `Authorization: Bearer <token>` header. token is set in `api.auth.auth_token` config. gets admin access
3. **Basic auth** — `Authorization: Basic <base64>` header. checked against `api.auth.users` in config. role depends on the user
4. **Query param** — `?token=<token>` for SSE connections where you cant set headers easily

if no auth is configured (no token, no users), everything is wide open with admin access. fine for dev, terrible for production

### Roles

- **admin** — full read/write access to everything
- **viewer** — read-only access. can see leases, config, status etc but cant delete or modify anything

endpoints that modify state require admin. everything else requires at minimum viewer

### Error format

all errors come back as:
```json
{
  "error": "human readable message",
  "code": "machine_readable_code"
}
```

---

## Endpoints

### Auth

#### POST /api/v2/auth/login
Login with username and password. returns a session cookie

```json
{"username": "admin", "password": "secret"}
```

response sets an `athena_session` cookie and returns user info:
```json
{"username": "admin", "role": "admin"}
```

#### POST /api/v2/auth/logout
Clears the session cookie

#### GET /api/v2/auth/me
Returns the currently authenticated user info

---

### Health

#### GET /api/v2/health
No auth required. use this for load balancer health checks

```json
{
  "status": "ok",
  "timestamp": 1706000000,
  "leases": 42
}
```

---

### Leases

#### GET /api/v2/leases
List all leases. supports filtering and pagination

**Query params:**
| Param | Description |
|-------|-------------|
| `subnet` | Filter by subnet CIDR e.g. `192.168.1.0/24` |
| `mac` | Filter by MAC (substring match, case insensitive) |
| `hostname` | Filter by hostname (substring match) |
| `state` | Filter by state: `offered`, `active`, `expired` |
| `limit` | Max results to return |
| `offset` | Skip this many results (for pagination) |

Response includes `X-Total-Count` header with total matching leases before pagination

```json
[
  {
    "ip": "192.168.1.100",
    "mac": "aa:bb:cc:dd:ee:ff",
    "client_id": "01aabbccddeeff",
    "hostname": "bobs-laptop",
    "subnet": "192.168.1.0/24",
    "pool": "192.168.1.100-192.168.1.200",
    "state": "active",
    "start": 1706000000,
    "expiry": 1706043200,
    "remaining_seconds": 3600,
    "last_updated": 1706040000
  }
]
```

#### GET /api/v2/leases/{ip}
Get a single lease by IP address

#### DELETE /api/v2/leases/{ip}
Delete (force-release) a lease. **admin only**

#### GET /api/v2/leases/export
Export all leases as CSV. response is `text/csv` with a download header

columns: `ip, mac, client_id, hostname, subnet, pool, state, start, expiry, remaining_seconds`

---

### Reservations

#### GET /api/v2/reservations
List all reservations across all subnets

```json
[
  {
    "id": 0,
    "subnet_index": 0,
    "subnet": "192.168.1.0/24",
    "mac": "00:11:22:33:44:55",
    "ip": "192.168.1.10",
    "hostname": "printer",
    "ddns_hostname": "printer.office.example.com"
  }
]
```

the `id` is a global sequential ID across all subnets. `subnet_index` tells you which `[[subnet]]` block it belongs to

#### POST /api/v2/reservations
Create a new reservation. **admin only**

```json
{
  "subnet_index": 0,
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.50",
  "hostname": "new-device"
}
```

either `mac` or `identifier` is required. `ip` is required. `subnet_index` must be valid

#### PUT /api/v2/reservations/{id}
Update a reservation by its global ID. **admin only**. only fields you include get updated

#### DELETE /api/v2/reservations/{id}
Delete a reservation. **admin only**

#### POST /api/v2/reservations/import
Bulk import reservations from CSV. **admin only**

POST body should be raw CSV with header: `subnet_index,mac,identifier,ip,hostname`

returns:
```json
{
  "imported": 15,
  "errors": ["row 3: invalid IP \"not.an.ip\""]
}
```

#### GET /api/v2/reservations/export
Export all reservations as CSV

---

### Subnets & Pools

#### GET /api/v2/subnets
List configured subnets

```json
[
  {
    "network": "192.168.1.0/24",
    "routers": ["192.168.1.1"],
    "dns_servers": ["192.168.1.1", "8.8.8.8"],
    "domain_name": "office.example.com",
    "lease_time": "12h",
    "pools": 2,
    "reservations": 3
  }
]
```

#### GET /api/v2/pools
Pool utilization stats

```json
[
  {
    "name": "pool-192.168.1.100",
    "range": "192.168.1.100-192.168.1.200",
    "size": 101,
    "allocated": 42,
    "available": 59,
    "utilization_percent": 41.58
  }
]
```

---

### Conflicts

#### GET /api/v2/conflicts
List active IP conflicts

#### GET /api/v2/conflicts/{ip}
Get conflict details for a specific IP

#### DELETE /api/v2/conflicts/{ip}
Clear a conflict entry — makes the IP available for allocation again. **admin only**

#### POST /api/v2/conflicts/{ip}/exclude
Permanently exclude an IP from allocation. **admin only**. the IP will never be handed out until you manually clear it

#### GET /api/v2/conflicts/history
Recently resolved conflicts

#### GET /api/v2/conflicts/stats
Conflict statistics broken down by detection method and subnet

```json
{
  "enabled": true,
  "active_count": 3,
  "resolved_count": 12,
  "permanent_count": 1,
  "by_method": {
    "arp_probe": 2,
    "client_decline": 1
  },
  "by_subnet": {
    "192.168.1.0/24": 3
  }
}
```

---

### Configuration

all dynamic config sections are stored in the database and exposed via REST endpoints. each section has a `GET` to read and a `PUT` to update

#### GET /api/v2/config/subnets
List all configured subnets from the database

#### POST /api/v2/config/subnets
Create a new subnet. **admin only**

#### PUT /api/v2/config/subnets/{network}
Update a subnet by CIDR. **admin only**

#### DELETE /api/v2/config/subnets/{network}
Delete a subnet. **admin only**

#### GET/PUT /api/v2/config/defaults
Global default DHCP options

#### GET/PUT /api/v2/config/conflict
Conflict detection settings

#### GET/PUT /api/v2/config/ha
HA settings. GET reads from TOML. PUT writes directly to the TOML file

#### GET/PUT /api/v2/config/hooks
Event hook configuration

#### GET/PUT /api/v2/config/ddns
Dynamic DNS configuration

#### GET/PUT /api/v2/config/dns
DNS proxy configuration

#### GET/PUT /api/v2/config/syslog
Remote syslog forwarding configuration

#### GET/PUT /api/v2/config/fingerprint
Device fingerprinting configuration (including Fingerbank API key)

#### GET/PUT /api/v2/config/hostname-sanitisation
Hostname sanitisation settings

#### POST /api/v2/config/import
Import a full TOML config into the database. **admin only**

```json
{"toml": "[conflict_detection]\nenabled = true\n..."}
```

#### GET /api/v2/config/raw
Running config as raw TOML text

#### POST /api/v2/config/validate
Validate config without applying

---

### Events & Hooks

#### GET /api/v2/events
List recent events

#### GET /api/v2/events/stream
**SSE (Server-Sent Events) endpoint**. streams live events as `text/event-stream`

each event is a JSON object:
```json
{
  "event": "lease.ack",
  "timestamp": "2024-01-23T14:30:22Z",
  "lease": {
    "ip": "192.168.1.100",
    "mac": "aa:bb:cc:dd:ee:ff",
    "hostname": "bobs-laptop",
    "subnet": "192.168.1.0/24"
  }
}
```

for auth, pass the token as a query param: `/api/v2/events/stream?token=mytoken`

#### GET /api/v2/hooks
List configured hooks and their status

#### POST /api/v2/hooks/test
Fire a test event through the event bus. **admin only**. useful for verifying your webhook URLs actually work

---

### Audit Log

#### GET /api/v2/audit
Query the audit log. supports filtering by time range, event type, user, IP, MAC

**Query params:**
| Param | Description |
|-------|-------------|
| `from` | Start time (RFC 3339 or unix timestamp) |
| `to` | End time |
| `type` | Event type filter |
| `user` | Username filter |
| `limit` | Max results |
| `offset` | Pagination offset |

#### GET /api/v2/audit/export
Export audit log as CSV. same query params as above

#### GET /api/v2/audit/stats
Audit activity breakdown by event type and time period

---

### Device Fingerprints

#### GET /api/v2/fingerprints
List all known device fingerprints

#### GET /api/v2/fingerprints/{mac}
Get fingerprint for a specific MAC address

#### GET /api/v2/fingerprints/stats
Fingerprint statistics: total devices, breakdown by type and OS, whether Fingerbank API key is configured

---

### Rogue DHCP Server Detection

#### GET /api/v2/rogue
List detected rogue DHCP servers

#### GET /api/v2/rogue/stats
Rogue detection statistics

#### POST /api/v2/rogue/scan
Trigger an active scan for rogue servers. **admin only**

#### POST /api/v2/rogue/acknowledge
Acknowledge a rogue server (mark as known). **admin only**

#### POST /api/v2/rogue/remove
Remove a rogue server entry. **admin only**

---

### Topology

#### GET /api/v2/topology
Network topology tree built from relay agent data (option 82)

#### GET /api/v2/topology/stats
Topology statistics

#### POST /api/v2/topology/label
Set a custom label on a topology node (switch, port). **admin only**

---

### Anomaly Detection

#### GET /api/v2/anomaly/weather
Current anomaly detection status. returns a weather-style indicator (calm, advisory, watch, warning, critical)

---

### MAC Vendor Lookup

#### GET /api/v2/macvendor/{mac}
Look up the manufacturer for a MAC address from the built-in OUI database

#### GET /api/v2/macvendor/stats
OUI database statistics

---

### RADIUS

#### GET /api/v2/radius
List RADIUS configurations per subnet

#### GET /api/v2/radius/{subnet}
Get RADIUS config for a specific subnet

#### PUT /api/v2/radius/{subnet}
Set RADIUS config for a subnet. **admin only**

#### DELETE /api/v2/radius/{subnet}
Remove RADIUS config for a subnet. **admin only**

#### POST /api/v2/radius/test
Test RADIUS connectivity. **admin only**

---

### Port Automation

#### GET /api/v2/portauto/rules
Get port automation rules

#### PUT /api/v2/portauto/rules
Set port automation rules. **admin only**

#### POST /api/v2/portauto/test
Test port automation rules. **admin only**

---

### Backup & Restore

#### GET /api/v2/backup
Export a full backup (database + config). **admin only**

#### POST /api/v2/backup/restore
Restore from a backup. **admin only**

---

### Setup Wizard

#### GET /api/v2/setup/status
Get setup wizard status (complete or not). no auth required

#### POST /api/v2/setup/ha
Configure HA during setup. locked after setup complete

#### POST /api/v2/setup/config
Apply initial config during setup. locked after setup complete

#### POST /api/v2/setup/complete
Mark setup as complete. locked after setup complete

---

### HA Status

#### GET /api/v2/ha/status
Current HA state

```json
{
  "enabled": true,
  "role": "primary",
  "state": "ACTIVE",
  "is_active": true,
  "last_heartbeat": "2024-01-23T14:30:22Z",
  "peer_address": "192.168.1.2:8067",
  "listen_address": "0.0.0.0:8067",
  "vip": {
    "configured": true,
    "active": true,
    "entries": [
      {
        "ip": "10.0.0.3",
        "cidr": 24,
        "interface": "eth0",
        "label": "DNS VIP",
        "held": true,
        "on_local": true,
        "acquired_at": "2024-01-23T14:25:00Z"
      }
    ]
  }
}
```

if HA is disabled, returns `{"enabled": false}`. the `vip` key is only present when VIPs are configured

#### POST /api/v2/ha/failover
Trigger manual failover — forces this node to ACTIVE state. **admin only**

be careful with this one

---

### Floating VIPs

floating virtual IPs managed by the active HA node. configured via the web UI or API, stored in the database, synced between peers

#### GET /api/v2/vips
Get the configured VIP entries

```json
[
  {"ip": "10.0.0.3", "cidr": 24, "interface": "eth0", "label": "DNS VIP"},
  {"ip": "10.1.0.3", "cidr": 24, "interface": "eth0.10", "label": "VLAN 10 DNS"}
]
```

returns `[]` if no VIPs configured

#### PUT /api/v2/vips
Replace the entire VIP list. **admin only**. the VIP group is hot-reloaded — old VIPs not in the new list are released, new ones are acquired if the node is active

request body is the same format as the GET response

#### GET /api/v2/vips/status
Runtime status of all VIPs. shows whether each VIP is currently held and present on the interface

```json
{
  "configured": true,
  "active": true,
  "entries": [
    {
      "ip": "10.0.0.3",
      "cidr": 24,
      "interface": "eth0",
      "label": "DNS VIP",
      "held": true,
      "on_local": true,
      "acquired_at": "2024-01-23T14:25:00Z"
    }
  ]
}
```

`held` means the VIP manager believes it owns the IP. `on_local` is a live check of whether the IP is actually on the interface. if `error` is present, something went wrong acquiring/releasing

---

### DNS Proxy

#### GET /api/v2/dns/stats
DNS proxy statistics: queries, cache, filter lists

#### GET /api/v2/dns/records
List local zone records (from DHCP leases and static records)

#### POST /api/v2/dns/cache/flush
Flush the DNS response cache. **admin only**

#### GET /api/v2/dns/lists
Filter list status (loaded, domain count, last refresh)

#### POST /api/v2/dns/lists/refresh
Force refresh all filter lists. **admin only**

#### POST /api/v2/dns/lists/test
Test a domain against the filter lists

```json
{"domain": "ads.example.com"}
```

#### GET /api/v2/dns/querylog
Recent DNS queries

#### GET /api/v2/dns/querylog/stream
SSE stream of live DNS queries

---

### Stats

#### GET /api/v2/stats
Server statistics summary

```json
{
  "leases": {"total": 42},
  "pools": 2,
  "subnets": 1,
  "conflicts": {"active": 3, "permanent": 1},
  "timestamp": 1706000000
}
```

---

### Prometheus Metrics

#### GET /metrics
Prometheus metrics endpoint. no auth required. see [monitoring.md](monitoring.md) for the full list of metrics
