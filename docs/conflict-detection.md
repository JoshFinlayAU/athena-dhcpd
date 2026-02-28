# Conflict Detection

this is probably the feature that makes athena-dhcpd worth using over whatever else you were considering. before handing out an IP, the server actually checks if something is already using it. wild concept right

## how it works

when a client sends a DHCPDISCOVER and the server picks a candidate IP from the pool, instead of just blindly offering it, it probes the IP first:

1. **check the conflict table** — is this IP already known to be conflicted? skip it immediately
2. **check the probe cache** — did we recently probe this IP and it was clear? skip the probe, its fine
3. **ARP or ICMP probe** — actually ask "hey is anyone using this IP?"
4. **if conflict detected** — add to conflict table, skip this IP, try the next one
5. **if clear** — cache the result, proceed with DHCPOFFER

the whole thing runs within the `probe_timeout` window (default 500ms). if no reply comes back within that time, the IP is considered clear

## ARP vs ICMP

the detector automatically picks the right probe method:

- **ARP probe** — used when the candidate IP is on a directly-attached subnet (the server can see it via layer 2). requires raw sockets (CAP_NET_RAW)
- **ICMP ping** — used for remote/relayed subnets where ARP won't work. also needs raw sockets but less reliable since some devices don't respond to ping

the auto-selection is based on checking if the candidate IP falls within any of the local interface's subnets. if it does → ARP. if not → ICMP

if neither prober is available (no raw socket capability), the server logs a loud warning and proceeds without probing. reduced safety is better than not starting at all

## probe strategies

### sequential (default)
probes candidate IPs one at a time. if the first one conflicts, try the second, etc. up to `max_probes_per_discover` attempts

worst case latency: `max_probes_per_discover × probe_timeout`

good for: most deployments. predictable, easy to reason about

### parallel
probes multiple candidates simultaneously. returns the first clear one

worst case latency: one `probe_timeout` period regardless of how many conflicts

good for: networks with lots of conflicts (messy environments, migration scenarios) where you want to minimize offer latency

configure via **Configuration > Conflict Detection** in the web UI or `PUT /api/v2/config/conflict`:

```json
{
  "probe_strategy": "parallel",
  "parallel_probe_count": 4
}
```

## the conflict table

when a conflict is detected, the IP goes into the conflict table:

- stored in BoltDB (`conflicts/` bucket) — survives restarts
- cached in memory for O(1) lookup
- each entry tracks: IP, detection method, responder MAC, subnet, probe count, timestamp
- `conflict_hold_time` controls how long the IP stays flagged (default 1h)
- after hold time expires, the IP becomes available again

### permanent conflicts

if an IP hits `max_conflict_count` detections (default 3), its permanently flagged. this means something is definitely squatting on that IP and it needs human attention

permanently flagged IPs are never offered. you have to manually clear them via the API:
```bash
# clear a single conflict
curl -X DELETE http://localhost:8067/api/v1/conflicts/192.168.1.50 -H "Authorization: Bearer mytoken"

# or permanently exclude it
curl -X POST http://localhost:8067/api/v1/conflicts/192.168.1.50/exclude -H "Authorization: Bearer mytoken"
```

or use the web UI conflicts page, which is honestly easier

![Conflicts](../screenshots/conflicts.png)

the conflict detection config is managed through the web UI Configuration page:

![Config — Conflict Detection](../screenshots/config_conflict_detection.png)

## DHCPDECLINE handling

if a client sends DHCPDECLINE (meaning the client itself detected a conflict after we offered the IP — oops), the IP gets added to the conflict table with `detection_method: "client_decline"`. the probe cache for that IP is immediately invalidated

this counts toward the permanent conflict threshold too

## gratuitous ARP

when `send_gratuitous_arp = true`, after a successful DHCPACK on a local subnet, the server sends a gratuitous ARP announcement. this updates ARP caches on switches and other devices, reducing the chance that the newly-assigned IP will have stale ARP entries pointing at the wrong MAC

only works on directly-attached subnets (same as ARP probing)

## probe cache

to avoid hammering the network with ARP/ICMP for the same IP over and over:

- after a successful (clear) probe, the result is cached for `probe_cache_ttl` (default 10s)
- during that window, the same IP won't be probed again
- DHCPDECLINE immediately invalidates the cache entry for that IP
- cache is concurrent-safe (sync.Map)

## events

conflict detection fires events through the event bus:

| Event | When |
|-------|------|
| `conflict.detected` | ARP/ICMP probe got a response |
| `conflict.decline` | Client sent DHCPDECLINE |
| `conflict.resolved` | Conflict hold time expired |
| `conflict.permanent` | IP exceeded max_conflict_count |

all of these are available to script hooks, webhooks, and the WebSocket event stream. conflict events include `ATHENA_CONFLICT_METHOD` and `ATHENA_CONFLICT_RESPONDER_MAC` environment variables for script hooks

## capability requirements

ARP probing uses raw sockets which need CAP_NET_RAW. three ways to get this:

```bash
# option 1: setcap on the binary
sudo setcap cap_net_raw+ep ./athena-dhcpd

# option 2: run as root (not recommended)
sudo ./athena-dhcpd

# option 3: systemd with AmbientCapabilities (recommended)
# already set up in deploy/athena-dhcpd.service
```

if the server cant open raw sockets, it logs a warning and disables probing. DHCP still works, you just dont get conflict detection

## metrics

all exposed via prometheus at `/metrics`:

- `athena_dhcpd_conflict_probes_total{method,result}` — probe counts by method (arp/icmp) and result (clear/conflict/error)
- `athena_dhcpd_conflict_probe_duration_seconds{method}` — probe latency histogram
- `athena_dhcpd_conflicts_active{subnet}` — current active conflicts
- `athena_dhcpd_conflicts_permanent{subnet}` — permanently flagged IPs
- `athena_dhcpd_conflict_declines_total{subnet}` — DHCPDECLINE counts
- `athena_dhcpd_probe_cache_hits_total` — cache hit rate
- `athena_dhcpd_probe_cache_misses_total` — cache miss rate
