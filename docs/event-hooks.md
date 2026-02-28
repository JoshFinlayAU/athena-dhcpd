# Event Hooks

things happen on your DHCP server. clients get IPs, conflicts get detected, failovers fire. you probably want to know about some of them. maybe update a CMDB, ping a slack channel, write to a syslog, feed a monitoring system, whatever

athena-dhcpd has two types of hooks: **scripts** and **webhooks**. both are driven by the same event bus. hook failures never affect DHCP processing — if your slack webhook is down, leases still get handed out. thats the deal

![Events](../screenshots/events.png)

hook configuration is managed through the web UI Configuration page:

![Config — Hooks](../screenshots/config_hooks.png)

## the event bus

all events flow through a buffered Go channel. the buffer size is configurable (`event_buffer_size`, default 10000). if the buffer fills up (your hooks are too slow), events get dropped with a warning log and a metric increment. the DHCP hot path never blocks waiting for hooks

subscribers (the hook dispatcher, the WebSocket hub, the DDNS manager) each get their own buffered channel. slow consumers get their events dropped independently — one slow webhook doesn't affect WebSocket streaming

## event types

| Event | When it fires |
|-------|---------------|
| `lease.discover` | Client sent DHCPDISCOVER |
| `lease.offer` | Server sent DHCPOFFER |
| `lease.ack` | Server sent DHCPACK (lease confirmed) |
| `lease.renew` | Lease renewed |
| `lease.nak` | Server sent DHCPNAK |
| `lease.release` | Client released its lease |
| `lease.decline` | Client sent DHCPDECLINE |
| `lease.expire` | Lease expired (GC cleaned it up) |
| `conflict.detected` | ARP/ICMP probe found a conflict |
| `conflict.decline` | Client-reported conflict via DHCPDECLINE |
| `conflict.resolved` | Conflict hold time expired, IP available again |
| `conflict.permanent` | IP exceeded max conflict count |
| `ha.failover` | HA state transition |
| `ha.sync_complete` | Bulk sync finished |

## event payload

every event is a JSON object with this structure:

```json
{
  "event": "lease.ack",
  "timestamp": "2024-01-23T14:30:22Z",
  "lease": {
    "ip": "192.168.1.100",
    "mac": "aa:bb:cc:dd:ee:ff",
    "client_id": "01aabbccddeeff",
    "hostname": "bobs-laptop",
    "fqdn": "bobs-laptop.example.com",
    "subnet": "192.168.1.0/24",
    "pool": "192.168.1.100-200",
    "start": 1706000000,
    "expiry": 1706043200,
    "state": "active",
    "relay": {
      "giaddr": "192.168.1.1",
      "circuit_id": "eth0/1/3",
      "remote_id": "switch01"
    }
  },
  "server": {
    "node_id": "dhcp1",
    "ha_role": "primary"
  },
  "reason": "optional reason string"
}
```

conflict events have a `conflict` field instead of/alongside `lease`:

```json
{
  "event": "conflict.detected",
  "conflict": {
    "ip": "192.168.1.50",
    "subnet": "192.168.1.0/24",
    "detection_method": "arp_probe",
    "responder_mac": "de:ad:be:ef:ca:fe",
    "probe_count": 2
  }
}
```

## script hooks

scripts are executed via `/bin/sh -c` with a configurable concurrency pool (default 4 workers) and timeout

### configuration

```toml
[[hooks.script]]
name = "log-leases"
events = ["lease.ack", "lease.release", "lease.expire"]
command = "/usr/local/bin/athena-hook.sh"
timeout = "10s"
subnets = ["192.168.1.0/24"]   # optional — only fire for these subnets
```

### event matching

the `events` field supports:
- exact match: `"lease.ack"`
- wildcard suffix: `"lease.*"` matches all lease events
- catch-all: `"*"` matches everything
- empty list: also matches everything (no filter = match all)

### how data gets to your script

two ways, simultaneously:

**1. Environment variables** (ATHENA_* prefix)

| Variable | Description |
|----------|-------------|
| `ATHENA_EVENT` | Event type e.g. `lease.ack` |
| `ATHENA_HOOK_NAME` | Hook name from config |
| `ATHENA_IP` | Lease or conflict IP |
| `ATHENA_MAC` | Client MAC address |
| `ATHENA_HOSTNAME` | Client hostname |
| `ATHENA_CLIENT_ID` | Client identifier |
| `ATHENA_SUBNET` | Subnet CIDR |
| `ATHENA_POOL` | Pool name |
| `ATHENA_FQDN` | Fully qualified domain name |
| `ATHENA_LEASE_START` | Lease start (unix timestamp) |
| `ATHENA_LEASE_EXPIRY` | Lease expiry (unix timestamp) |
| `ATHENA_LEASE_DURATION` | Lease duration in seconds |
| `ATHENA_OLD_IP` | Previous IP (for renewals with IP change) |
| `ATHENA_GATEWAY` | Relay agent gateway IP |
| `ATHENA_RELAY_AGENT_CIRCUIT_ID` | Relay circuit ID |
| `ATHENA_RELAY_AGENT_REMOTE_ID` | Relay remote ID |
| `ATHENA_DNS_SERVERS` | Assigned DNS servers |
| `ATHENA_DOMAIN` | Assigned domain name |
| `ATHENA_SERVER_ID` | Server node ID |
| `ATHENA_CONFLICT_METHOD` | Conflict detection method |
| `ATHENA_CONFLICT_RESPONDER_MAC` | MAC that responded to the probe |

**2. JSON on stdin**

the full event JSON is piped to the script's stdin. useful if you need access to nested fields or want to parse it with jq:

```bash
#!/bin/bash
# read the full event JSON
EVENT=$(cat)

# or use env vars for the simple stuff
echo "Lease ${ATHENA_EVENT}: ${ATHENA_IP} -> ${ATHENA_MAC}"

# parse JSON for complex stuff
echo "$EVENT" | jq '.lease.relay.circuit_id'
```

### script behavior

- runs in a bounded goroutine pool (semaphore). if all workers are busy, the next script execution is dropped with a warning
- timeout is always enforced. if the script doesn't finish in time, it gets killed (SIGKILL via context cancellation)
- stdout and stderr are captured — stderr is logged on failure
- exit code is logged at debug level on success, error level on failure
- scripts inherit the server's environment plus the ATHENA_* variables

### example: update a CMDB

```bash
#!/bin/bash
# /usr/local/bin/update-cmdb.sh
case "$ATHENA_EVENT" in
  lease.ack)
    curl -s -X POST "https://cmdb.internal/api/hosts" \
      -H "Content-Type: application/json" \
      -d "{\"ip\": \"$ATHENA_IP\", \"mac\": \"$ATHENA_MAC\", \"hostname\": \"$ATHENA_HOSTNAME\"}"
    ;;
  lease.release|lease.expire)
    curl -s -X DELETE "https://cmdb.internal/api/hosts/$ATHENA_IP"
    ;;
esac
```

## webhook hooks

HTTP webhooks with retries, backoff, HMAC signing, and built-in templates for slack and teams

### configuration

```toml
[[hooks.webhook]]
name = "slack-alerts"
events = ["conflict.detected", "conflict.permanent", "ha.failover"]
url = "https://hooks.slack.com/services/T00/B00/XXXXX"
method = "POST"
timeout = "5s"
retries = 3
retry_backoff = "1s"
template = "slack"
secret = "my-hmac-secret"

[hooks.webhook.headers]
X-Custom-Header = "whatever"
```

### request format

webhooks send HTTP requests with:

- `Content-Type: application/json`
- `X-Athena-Event: dhcp-event`
- `User-Agent: athena-dhcpd/1.0`
- any custom headers from config
- if `secret` is set: `X-Athena-Signature: sha256=<hex-hmac>`

### HMAC signature verification

if you set a `secret`, every request gets an `X-Athena-Signature` header containing `sha256=` followed by the hex-encoded HMAC-SHA256 of the request body. verify it on the receiving end:

```python
import hmac, hashlib

def verify(body, signature, secret):
    expected = 'sha256=' + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(signature, expected)
```

### retry behavior

on failure (non-2xx response or network error), retries with exponential backoff:

- attempt 1: immediate
- attempt 2: after `retry_backoff` (default 2s)
- attempt 3: after `retry_backoff * 2` (4s)
- etc

if all retries fail, logs an error and moves on. never blocks DHCP

### templates

#### `"slack"`
formats the event as a Slack-compatible `{"text": "..."}` payload with markdown formatting

#### `"teams"`
formats as a Microsoft Teams MessageCard with title, theme color, and HTML formatting

#### empty / not set
sends the raw event JSON. use this for custom integrations

### testing webhooks

hit the test endpoint:
```bash
curl -X POST http://localhost:8080/api/v1/hooks/test \
  -H "Authorization: Bearer mytoken"
```

this fires a fake `lease.ack` event through the bus, which triggers any hooks that match `lease.ack` or `lease.*` or `*`

## metrics

- `athena_dhcpd_hook_executions_total{hook_type,result}` — execution counts by type (script/webhook) and result (success/error)
- `athena_dhcpd_hook_execution_duration_seconds{hook_type}` — execution latency histogram
- `athena_dhcpd_events_published_total{event_type}` — events published to the bus
- `athena_dhcpd_event_buffer_drops_total` — events dropped due to full buffer (if this is nonzero, increase `event_buffer_size` or make your hooks faster)
