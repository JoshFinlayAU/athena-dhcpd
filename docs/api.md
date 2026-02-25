# API Reference

the REST API runs on whatever you set `api.listen` to (default `0.0.0.0:8067`). all responses are JSON unless noted otherwise

## Authentication

three ways to authenticate:

1. **Bearer token** — `Authorization: Bearer <token>` header. token is set in `api.auth.auth_token` config. gets admin access
2. **Basic auth** — `Authorization: Basic <base64>` header. checked against `api.auth.users` in config. role depends on the user
3. **Query param** — `?token=<token>` for WebSocket connections where you cant set headers easily

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

### Health

#### GET /api/v1/health
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

#### GET /api/v1/leases
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

#### GET /api/v1/leases/{ip}
Get a single lease by IP address

#### DELETE /api/v1/leases/{ip}
Delete (force-release) a lease. **admin only**

#### GET /api/v1/leases/export
Export all leases as CSV. response is `text/csv` with a download header

columns: `ip, mac, client_id, hostname, subnet, pool, state, start, expiry, remaining_seconds`

---

### Reservations

#### GET /api/v1/reservations
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

#### POST /api/v1/reservations
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

#### PUT /api/v1/reservations/{id}
Update a reservation by its global ID. **admin only**. only fields you include get updated

#### DELETE /api/v1/reservations/{id}
Delete a reservation. **admin only**

#### POST /api/v1/reservations/import
Bulk import reservations from CSV. **admin only**

POST body should be raw CSV with header: `subnet_index,mac,identifier,ip,hostname`

returns:
```json
{
  "imported": 15,
  "errors": ["row 3: invalid IP \"not.an.ip\""]
}
```

#### GET /api/v1/reservations/export
Export all reservations as CSV

---

### Subnets & Pools

#### GET /api/v1/subnets
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

#### GET /api/v1/pools
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

#### GET /api/v1/conflicts
List active IP conflicts

#### GET /api/v1/conflicts/{ip}
Get conflict details for a specific IP

#### DELETE /api/v1/conflicts/{ip}
Clear a conflict entry — makes the IP available for allocation again. **admin only**

#### POST /api/v1/conflicts/{ip}/exclude
Permanently exclude an IP from allocation. **admin only**. the IP will never be handed out until you manually clear it

#### GET /api/v1/conflicts/history
Recently resolved conflicts

#### GET /api/v1/conflicts/stats
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

#### GET /api/v1/config
Running config as JSON (secrets redacted). TSIG secrets, API keys, password hashes all show as `"***REDACTED***"`

#### GET /api/v1/config/raw
Running config as raw TOML text

#### PUT /api/v1/config
Write new config. **admin only**. body is JSON with a `config` field containing the TOML:

```json
{
  "config": "[server]\ninterface = \"eth0\"\n..."
}
```

this will:
1. Validate the TOML
2. Create a timestamped backup (0600 permissions)
3. Write to a temp file
4. Atomic rename over the old config

returns the backup filename so you can restore if needed

#### POST /api/v1/config/validate
Validate config without applying. same body format as PUT. returns:

```json
{
  "valid": true,
  "errors": []
}
```

#### GET /api/v1/config/backups
List available config backups (newest first)

```json
[
  {
    "name": "config.toml.bak.20240123T143022",
    "size": 2048,
    "modified": "2024-01-23T14:30:22Z"
  }
]
```

#### GET /api/v1/config/backups/{timestamp}
Download a specific backup as TOML text

---

### Events & Hooks

#### GET /api/v1/events
List recent events (placeholder — will return persisted events in a future version)

#### GET /api/v1/events/stream
**WebSocket endpoint**. upgrades to a WebSocket connection that streams live events as JSON

each message is a JSON event object:
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

the server sends ping frames every 30 seconds. if you dont respond with pong within 60 seconds you get disconnected

for auth, pass the token as a query param: `/api/v1/events/stream?token=mytoken`

#### GET /api/v1/hooks
List configured hooks and their status

#### POST /api/v1/hooks/test
Fire a test event through the event bus. **admin only**. useful for verifying your webhook URLs actually work

---

### HA Status

#### GET /api/v1/ha/status
Current HA state

```json
{
  "enabled": true,
  "role": "primary",
  "state": "ACTIVE",
  "is_active": true,
  "last_heartbeat": "2024-01-23T14:30:22Z",
  "peer_address": "192.168.1.2:8067",
  "listen_address": "0.0.0.0:8067"
}
```

if HA is disabled, returns `{"enabled": false}`

#### POST /api/v1/ha/failover
Trigger manual failover — forces this node to ACTIVE state. **admin only**

be careful with this one

---

### Stats

#### GET /api/v1/stats
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
