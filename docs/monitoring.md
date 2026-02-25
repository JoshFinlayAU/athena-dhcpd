# Monitoring

Prometheus metrics at `/metrics`. no auth on this endpoint so your prometheus scraper doesn't need to deal with tokens

## scrape config

```yaml
scrape_configs:
  - job_name: 'athena-dhcpd'
    static_configs:
      - targets: ['dhcp-server:8080']
    scrape_interval: 15s
```

all metrics use the `athena_dhcpd_` prefix

## metrics reference

### DHCP packets

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `packets_received_total` | counter | `msg_type` | Packets received by message type (discover, request, release, decline, inform) |
| `packets_sent_total` | counter | `msg_type` | Packets sent by message type (offer, ack, nak) |
| `packet_errors_total` | counter | `type` | Packet processing errors by error type |
| `packet_processing_duration_seconds` | histogram | `msg_type` | How long it takes to process each packet type. buckets from 0.1ms to 1s |

useful queries:
```promql
# DHCP requests per second
rate(athena_dhcpd_packets_received_total[5m])

# offer latency p99 (includes conflict probe time)
histogram_quantile(0.99, rate(athena_dhcpd_packet_processing_duration_seconds_bucket{msg_type="discover"}[5m]))
```

### leases

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `leases_active` | gauge | | Currently active leases |
| `leases_offered` | gauge | | Currently offered (pending) leases |
| `lease_operations_total` | counter | `operation` | Lease state transitions (offer, ack, renew, release, decline, expire) |

```promql
# lease churn rate
rate(athena_dhcpd_lease_operations_total{operation="ack"}[5m])

# active lease count
athena_dhcpd_leases_active
```

### pools

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pool_size` | gauge | `subnet`, `pool` | Total IPs in each pool |
| `pool_allocated` | gauge | `subnet`, `pool` | Allocated IPs in each pool |
| `pool_utilization_percent` | gauge | `subnet`, `pool` | Utilization as a percentage |
| `pool_exhausted_total` | counter | `subnet` | Times a pool was completely full during allocation |

```promql
# pools above 90% utilization (time to expand)
athena_dhcpd_pool_utilization_percent > 90

# pool exhaustion events (something is wrong)
rate(athena_dhcpd_pool_exhausted_total[1h]) > 0
```

### conflict detection

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `conflict_probes_total` | counter | `method`, `result` | Probes by method (arp_probe, icmp_probe) and result (clear, conflict, error) |
| `conflict_probe_duration_seconds` | histogram | `method` | Probe latency. buckets from 1ms to 2s |
| `conflicts_active` | gauge | `subnet` | Currently active conflicts |
| `conflicts_permanent` | gauge | `subnet` | Permanently flagged IPs |
| `conflicts_resolved_total` | counter | `method` | Resolved conflicts |
| `conflict_declines_total` | counter | `subnet` | DHCPDECLINE events |
| `probe_cache_hits_total` | counter | | Probe cache hits |
| `probe_cache_misses_total` | counter | | Probe cache misses |

```promql
# conflict rate
rate(athena_dhcpd_conflict_probes_total{result="conflict"}[5m])

# probe cache hit ratio
athena_dhcpd_probe_cache_hits_total / (athena_dhcpd_probe_cache_hits_total + athena_dhcpd_probe_cache_misses_total)

# permanently conflicted IPs (needs attention)
athena_dhcpd_conflicts_permanent > 0
```

### event bus & hooks

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `events_published_total` | counter | `event_type` | Events published to the bus |
| `event_buffer_drops_total` | counter | | Events dropped (buffer full) |
| `hook_executions_total` | counter | `hook_type`, `result` | Hook executions by type (script, webhook) and result (success, error) |
| `hook_execution_duration_seconds` | histogram | `hook_type` | Hook execution latency |

```promql
# event drops (bad — increase buffer or fix slow hooks)
rate(athena_dhcpd_event_buffer_drops_total[5m]) > 0

# webhook failure rate
rate(athena_dhcpd_hook_executions_total{hook_type="webhook",result="error"}[5m])
```

### high availability

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ha_state` | gauge | `state`, `role` | Current HA state (1 = active for that state+role combo) |
| `ha_heartbeats_sent_total` | counter | | Heartbeats sent to peer |
| `ha_heartbeats_received_total` | counter | | Heartbeats received from peer |
| `ha_sync_operations_total` | counter | `type` | Sync operations (lease_update, conflict_update) |
| `ha_sync_errors_total` | counter | | Sync failures |

```promql
# is the peer alive? (heartbeats should be ~1/sec)
rate(athena_dhcpd_ha_heartbeats_received_total[1m])

# sync errors (connection issues)
rate(athena_dhcpd_ha_sync_errors_total[5m]) > 0
```

### API

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `api_requests_total` | counter | `method`, `path`, `status` | HTTP requests by method, path, status code |
| `api_request_duration_seconds` | histogram | `method`, `path` | API request latency |
| `websocket_connections_active` | gauge | | Active WebSocket connections |

```promql
# API error rate
rate(athena_dhcpd_api_requests_total{status=~"5.."}[5m])

# API latency p95
histogram_quantile(0.95, rate(athena_dhcpd_api_request_duration_seconds_bucket[5m]))
```

### DDNS

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ddns_updates_total` | counter | `type`, `result` | DNS updates by type (add_a, add_ptr, remove_a, remove_ptr) and result |
| `ddns_update_duration_seconds` | histogram | `type` | DNS update latency |

```promql
# DNS update failure rate
rate(athena_dhcpd_ddns_updates_total{result="error"}[5m])
```

### server

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `server_info` | gauge | `version` | Server version (constant 1, version in label) |
| `server_start_time_seconds` | gauge | | Server start time as unix timestamp |

```promql
# uptime
time() - athena_dhcpd_server_start_time_seconds
```

## Grafana dashboard

no prebuilt dashboard yet (PRs welcome) but heres what I'd put on one:

### row 1: overview
- active leases gauge
- packets/sec graph (received vs sent)
- pool utilization per subnet
- uptime

### row 2: DHCP performance
- packet processing latency (p50, p95, p99) by message type
- DORA success rate
- rate limit drops (if you have rate limiting on)

### row 3: conflict detection
- conflict rate over time
- active vs permanent conflicts
- probe latency by method
- cache hit ratio

### row 4: infrastructure
- HA state + heartbeat rate
- hook execution rate and errors
- DNS update rate and errors
- API request rate and latency
- WebSocket connections
- event buffer drops

## alerting suggestions

stuff you probably want to get paged for:

```yaml
groups:
  - name: athena-dhcpd
    rules:
      - alert: PoolNearlyFull
        expr: athena_dhcpd_pool_utilization_percent > 90
        for: 5m
        annotations:
          summary: "DHCP pool {{ $labels.subnet }}/{{ $labels.pool }} is {{ $value }}% full"

      - alert: PoolExhausted
        expr: rate(athena_dhcpd_pool_exhausted_total[5m]) > 0
        annotations:
          summary: "DHCP pool exhaustion in subnet {{ $labels.subnet }}"

      - alert: PermanentConflicts
        expr: athena_dhcpd_conflicts_permanent > 0
        annotations:
          summary: "{{ $value }} permanently conflicted IPs in {{ $labels.subnet }}"

      - alert: HAPartnerDown
        expr: rate(athena_dhcpd_ha_heartbeats_received_total[2m]) == 0
        for: 30s
        annotations:
          summary: "HA peer heartbeats stopped"

      - alert: EventBufferDrops
        expr: rate(athena_dhcpd_event_buffer_drops_total[5m]) > 0
        annotations:
          summary: "Event buffer dropping events — hooks may be too slow"

      - alert: HighConflictRate
        expr: rate(athena_dhcpd_conflict_probes_total{result="conflict"}[5m]) > 1
        for: 5m
        annotations:
          summary: "High IP conflict rate — possible rogue DHCP server or static IP chaos"
```

## structured logging

athena-dhcpd uses Go's `slog` for structured JSON logging. output goes to stdout (systemd captures it to journal). every log line includes relevant context fields:

```json
{
  "time": "2024-01-23T14:30:22.123Z",
  "level": "INFO",
  "msg": "DHCPACK",
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.100",
  "subnet": "192.168.1.0/24",
  "hostname": "bobs-laptop",
  "lease_time": "12h0m0s"
}
```

log levels:
- **debug** — probe results, heartbeats, cache hits, option details. very noisy
- **info** — DHCP transactions (DISCOVER, OFFER, ACK), startup, shutdown, config changes
- **warn** — conflicts detected, capability issues, retries, non-critical failures
- **error** — actual failures that need attention

set via config: `log_level = "info"` or change at runtime via SIGHUP config reload
