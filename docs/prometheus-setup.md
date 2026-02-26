# Prometheus Setup Guide

step by step guide to getting athena-dhcpd metrics into Prometheus and visualized in Grafana. covers both DHCP and DNS proxy metrics

## prerequisites

- athena-dhcpd running with the API enabled
- Prometheus installed (or docker)
- Grafana installed (optional, for dashboards)

athena-dhcpd exposes metrics at `http://<server>:8067/metrics` by default. no authentication required on this endpoint so your scraper just works

## installing prometheus

### debian/ubuntu

```bash
sudo apt install prometheus
```

### docker

```bash
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v /etc/prometheus:/etc/prometheus \
  prom/prometheus
```

### docker compose

```yaml
services:
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    restart: unless-stopped

volumes:
  prometheus-data:
```

## configuring prometheus to scrape athena-dhcpd

add this to your `/etc/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'athena-dhcpd'
    static_configs:
      - targets: ['10.0.0.1:8067']
    scrape_interval: 15s
```

if you're running HA with two nodes, scrape both:

```yaml
scrape_configs:
  - job_name: 'athena-dhcpd'
    static_configs:
      - targets:
          - '10.0.0.1:8067'
          - '10.0.0.2:8067'
        labels:
          env: 'production'
    scrape_interval: 15s
```

reload prometheus after changing config:

```bash
# systemd
sudo systemctl reload prometheus

# docker
docker kill -s SIGHUP prometheus

# or just restart it
sudo systemctl restart prometheus
```

### verify its working

open `http://localhost:9090/targets` in your browser. your athena-dhcpd target should show as UP

or from the command line:
```bash
curl -s http://10.0.0.1:8067/metrics | grep athena_dhcpd_
```

you should see a bunch of metrics with the `athena_dhcpd_` prefix

## DHCP metrics

these are the metrics you care about most for monitoring DHCP health

### queries to try in prometheus

```promql
# total DHCP packets per second by type
rate(athena_dhcpd_packets_received_total[5m])

# DHCPACK rate (successful lease assignments)
rate(athena_dhcpd_packets_sent_total{msg_type="ack"}[5m])

# active lease count
athena_dhcpd_leases_active

# pool utilization per subnet (most important metric)
athena_dhcpd_pool_utilization_percent

# pools that are getting full
athena_dhcpd_pool_utilization_percent > 80

# offer latency p99 (includes ARP probe time)
histogram_quantile(0.99, rate(athena_dhcpd_packet_processing_duration_seconds_bucket{msg_type="discover"}[5m]))

# lease churn — new leases per minute
rate(athena_dhcpd_lease_operations_total{operation="ack"}[5m]) * 60

# pool exhaustion events
increase(athena_dhcpd_pool_exhausted_total[1h])

# conflict detection rate
rate(athena_dhcpd_conflict_probes_total{result="conflict"}[5m])

# permanently conflicted IPs needing manual attention
athena_dhcpd_conflicts_permanent
```

## DNS proxy metrics

if you have the built-in DNS proxy enabled, these track its performance

### queries to try

```promql
# DNS queries per second by result
rate(athena_dhcpd_dns_queries_total[5m])

# total DNS queries per second
sum(rate(athena_dhcpd_dns_queries_total[5m]))

# blocked queries per second
rate(athena_dhcpd_dns_queries_total{status="blocked"}[5m])

# block rate as a percentage
sum(rate(athena_dhcpd_dns_queries_total{status="blocked"}[5m])) / sum(rate(athena_dhcpd_dns_queries_total[5m])) * 100

# cache hit ratio (higher is better, saves upstream bandwidth)
athena_dhcpd_dns_cache_hits_total / (athena_dhcpd_dns_cache_hits_total + athena_dhcpd_dns_cache_misses_total) * 100

# DNS query latency p95 by result type
histogram_quantile(0.95, rate(athena_dhcpd_dns_query_duration_seconds_bucket[5m]))

# upstream forward failures
rate(athena_dhcpd_dns_upstream_errors_total[5m])

# queries blocked per list (see which lists are doing work)
rate(athena_dhcpd_dns_blocked_total[5m])

# current cache size
athena_dhcpd_dns_cache_entries

# local zone record count
athena_dhcpd_dns_zone_records

# query breakdown by DNS type (A, AAAA, PTR, etc)
sum by (qtype) (rate(athena_dhcpd_dns_queries_total[5m]))
```

## DNS metrics reference

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dns_queries_total` | counter | `qtype`, `status` | Queries by DNS type (A, AAAA, etc) and result (local, cached, forwarded, blocked, failed) |
| `dns_query_duration_seconds` | histogram | `status` | Query processing latency. buckets from 0.1ms to 1s |
| `dns_cache_entries` | gauge | | Current entries in the response cache |
| `dns_cache_hits_total` | counter | | Cache hits |
| `dns_cache_misses_total` | counter | | Cache misses (query forwarded upstream) |
| `dns_blocked_total` | counter | `list`, `action` | Blocked queries by list name and action (nxdomain, zero, refuse) |
| `dns_zone_records` | gauge | | Records in the local zone (static + DHCP registrations) |
| `dns_upstream_errors_total` | counter | | Failed upstream forward attempts |

all metrics use the `athena_dhcpd_` prefix, so the full metric name is e.g. `athena_dhcpd_dns_queries_total`

## setting up grafana

### install grafana

```bash
# debian/ubuntu
sudo apt install grafana
sudo systemctl enable --now grafana-server

# docker
docker run -d \
  --name grafana \
  -p 3000:3000 \
  grafana/grafana
```

default login is admin/admin

### add prometheus data source

1. open grafana at `http://localhost:3000`
2. go to **Connections** → **Data sources** → **Add data source**
3. select **Prometheus**
4. set URL to `http://localhost:9090` (or your prometheus address)
5. click **Save & test**

### example dashboard panels

heres a set of panels to build a useful dashboard. each one is a prometheus query you paste into a new panel

#### row 1: DHCP overview

**Active Leases** (stat panel)
```promql
athena_dhcpd_leases_active
```

**DHCP Packets/sec** (time series)
```promql
sum by (msg_type) (rate(athena_dhcpd_packets_received_total[5m]))
```

**Pool Utilization** (gauge panel, one per subnet)
```promql
athena_dhcpd_pool_utilization_percent
```

**Server Uptime** (stat panel)
```promql
time() - athena_dhcpd_server_start_time_seconds
```

#### row 2: DHCP performance

**DORA Latency** (time series, p50/p95/p99)
```promql
histogram_quantile(0.50, rate(athena_dhcpd_packet_processing_duration_seconds_bucket{msg_type="discover"}[5m]))
histogram_quantile(0.95, rate(athena_dhcpd_packet_processing_duration_seconds_bucket{msg_type="discover"}[5m]))
histogram_quantile(0.99, rate(athena_dhcpd_packet_processing_duration_seconds_bucket{msg_type="discover"}[5m]))
```

**Lease Operations** (time series)
```promql
sum by (operation) (rate(athena_dhcpd_lease_operations_total[5m]))
```

**Conflict Probes** (time series)
```promql
sum by (result) (rate(athena_dhcpd_conflict_probes_total[5m]))
```

#### row 3: DNS proxy

**DNS Queries/sec** (time series, stacked)
```promql
sum by (status) (rate(athena_dhcpd_dns_queries_total[5m]))
```

**DNS Cache Hit Rate** (stat panel, percent)
```promql
rate(athena_dhcpd_dns_cache_hits_total[5m]) / (rate(athena_dhcpd_dns_cache_hits_total[5m]) + rate(athena_dhcpd_dns_cache_misses_total[5m])) * 100
```

**DNS Block Rate** (stat panel, percent)
```promql
sum(rate(athena_dhcpd_dns_queries_total{status="blocked"}[5m])) / sum(rate(athena_dhcpd_dns_queries_total[5m])) * 100
```

**DNS Query Latency** (time series, p50/p95)
```promql
histogram_quantile(0.50, sum by (le) (rate(athena_dhcpd_dns_query_duration_seconds_bucket[5m])))
histogram_quantile(0.95, sum by (le) (rate(athena_dhcpd_dns_query_duration_seconds_bucket[5m])))
```

**Blocked by List** (time series, stacked)
```promql
sum by (list) (rate(athena_dhcpd_dns_blocked_total[5m]))
```

**Upstream Errors** (time series)
```promql
rate(athena_dhcpd_dns_upstream_errors_total[5m])
```

#### row 4: infrastructure

**HA Heartbeats** (time series)
```promql
rate(athena_dhcpd_ha_heartbeats_received_total[1m])
```

**Hook Executions** (time series)
```promql
sum by (result) (rate(athena_dhcpd_hook_executions_total[5m]))
```

**API Requests** (time series)
```promql
sum by (status) (rate(athena_dhcpd_api_requests_total[5m]))
```

**Event Buffer Drops** (stat panel)
```promql
increase(athena_dhcpd_event_buffer_drops_total[1h])
```

## alerting rules

add these to your prometheus alerting rules file (usually `/etc/prometheus/rules/athena.yml`):

```yaml
groups:
  - name: athena-dhcpd-dhcp
    rules:
      - alert: DHCPPoolNearlyFull
        expr: athena_dhcpd_pool_utilization_percent > 90
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "DHCP pool {{ $labels.subnet }} at {{ $value | printf \"%.0f\" }}%"
          description: "Pool {{ $labels.pool }} in subnet {{ $labels.subnet }} is nearly full. expand the pool or check for leaks"

      - alert: DHCPPoolExhausted
        expr: rate(athena_dhcpd_pool_exhausted_total[5m]) > 0
        labels:
          severity: critical
        annotations:
          summary: "DHCP pool exhausted in {{ $labels.subnet }}"
          description: "clients are being denied IPs because the pool is full"

      - alert: IPConflictsPermanent
        expr: athena_dhcpd_conflicts_permanent > 0
        labels:
          severity: warning
        annotations:
          summary: "{{ $value }} permanent IP conflicts in {{ $labels.subnet }}"
          description: "these IPs have hit the max conflict count and wont be allocated. needs manual review"

      - alert: HighConflictRate
        expr: rate(athena_dhcpd_conflict_probes_total{result="conflict"}[10m]) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "high IP conflict rate detected"
          description: "possible rogue DHCP server or lots of static IPs in the pool range"

      - alert: HAPartnerDown
        expr: rate(athena_dhcpd_ha_heartbeats_received_total[2m]) == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "HA peer heartbeats stopped on {{ $labels.instance }}"

      - alert: EventBufferDropping
        expr: rate(athena_dhcpd_event_buffer_drops_total[5m]) > 0
        labels:
          severity: warning
        annotations:
          summary: "event buffer dropping events"
          description: "hooks might be too slow or buffer is too small"

  - name: athena-dhcpd-dns
    rules:
      - alert: DNSUpstreamFailures
        expr: rate(athena_dhcpd_dns_upstream_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "DNS upstream forward failures on {{ $labels.instance }}"
          description: "check if upstream forwarders are reachable"

      - alert: DNSCacheHitRateLow
        expr: |
          (rate(athena_dhcpd_dns_cache_hits_total[30m]) /
           (rate(athena_dhcpd_dns_cache_hits_total[30m]) + rate(athena_dhcpd_dns_cache_misses_total[30m])))
          < 0.3
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "DNS cache hit rate below 30%"
          description: "cache might be too small or TTLs are very short. current hit rate: {{ $value | printf \"%.0f\" }}%"

      - alert: DNSHighBlockRate
        expr: |
          sum(rate(athena_dhcpd_dns_queries_total{status="blocked"}[5m])) /
          sum(rate(athena_dhcpd_dns_queries_total[5m])) > 0.5
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "over 50% of DNS queries being blocked"
          description: "might be normal if you have aggressive blocklists, or a client might be malware-ing"
```

reference this rules file in your prometheus config:

```yaml
rule_files:
  - /etc/prometheus/rules/athena.yml
```

## alertmanager integration

if you want to actually get notified when alerts fire, set up alertmanager:

```yaml
# in prometheus.yml
alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']
```

basic alertmanager config for slack notifications:

```yaml
# /etc/alertmanager/alertmanager.yml
route:
  receiver: 'slack'
  group_by: ['alertname', 'instance']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h

receivers:
  - name: 'slack'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
```

## quick sanity check

after everything is set up, run through this checklist:

1. **metrics endpoint responds**
   ```bash
   curl -s http://10.0.0.1:8067/metrics | head -5
   ```

2. **prometheus target is UP**
   - open `http://prometheus:9090/targets`

3. **DHCP metrics are populating**
   ```promql
   athena_dhcpd_leases_active
   ```

4. **DNS metrics are populating** (if DNS proxy enabled)
   ```promql
   sum(athena_dhcpd_dns_queries_total)
   ```

5. **alerts are loaded**
   - open `http://prometheus:9090/alerts`
   - you should see the athena-dhcpd alert rules listed
