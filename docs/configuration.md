# Configuration Reference

athena-dhcpd uses TOML because its readable by humans. no YAML, no JSON with 47 levels of nesting. just TOML

the config file is passed via `-config` flag:
```bash
./athena-dhcpd -config /etc/athena-dhcpd/config.toml
```

hot-reload via SIGHUP — no restart needed for most changes:
```bash
kill -HUP $(cat /var/run/athena-dhcpd.pid)
```

see `configs/example.toml` for a fully annotated working example

---

## [server]

Core server settings. the basics

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `interface` | string | `"eth0"` | Default network interface. can be overridden per-subnet |
| `bind_address` | string | `"0.0.0.0:67"` | UDP bind address for DHCP |
| `server_id` | string | required | Server identifier IP (sent in option 54). usually your server's IP on the DHCP interface |
| `log_level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |
| `lease_db` | string | `"/var/lib/athena-dhcpd/leases.db"` | Path to BoltDB lease database |
| `pid_file` | string | `"/run/athena-dhcpd/athena-dhcpd.pid"` | PID file path. parent directory is created automatically. empty string disables |

```toml
[server]
interface = "eth0"
bind_address = "0.0.0.0:67"
server_id = "192.168.1.1"
log_level = "info"
lease_db = "/var/lib/athena-dhcpd/leases.db"
pid_file = "/run/athena-dhcpd/athena-dhcpd.pid"
```

### [server.rate_limit]

Rate limiting to prevent DHCP starvation attacks. one misbehaving client shouldn't be able to DOS your whole network

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable rate limiting |
| `max_discovers_per_second` | int | `100` | Global DISCOVER rate limit |
| `max_per_mac_per_second` | int | `5` | Per-MAC rate limit |

```toml
[server.rate_limit]
enabled = true
max_discovers_per_second = 100
max_per_mac_per_second = 10
```

---

## [conflict_detection]

IP conflict detection via ARP and ICMP probing. the server checks if an IP is already in use before handing it out. see [conflict-detection.md](conflict-detection.md) for the full story

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable conflict detection |
| `probe_strategy` | string | `"sequential"` | `"sequential"` or `"parallel"` |
| `probe_timeout` | duration | `"500ms"` | How long to wait for a probe reply |
| `max_probes_per_discover` | int | `3` | Max IPs to probe per DISCOVER |
| `parallel_probe_count` | int | `3` | How many IPs to probe simultaneously (parallel mode only) |
| `conflict_hold_time` | duration | `"1h"` | How long a conflicted IP stays flagged |
| `max_conflict_count` | int | `3` | After this many detections, IP is permanently flagged |
| `probe_cache_ttl` | duration | `"10s"` | How long a "clear" probe result is cached |
| `send_gratuitous_arp` | bool | `false` | Send gratuitous ARP after DHCPACK on local subnets |
| `icmp_fallback` | bool | `false` | Use ICMP ping when ARP isn't available |
| `probe_log_level` | string | `"debug"` | Log level for probe results |

```toml
[conflict_detection]
enabled = true
probe_strategy = "sequential"
probe_timeout = "500ms"
max_probes_per_discover = 3
conflict_hold_time = "1h"
max_conflict_count = 3
probe_cache_ttl = "10s"
send_gratuitous_arp = true
icmp_fallback = true
```

duration format: Go duration strings like `"500ms"`, `"10s"`, `"1h"`, `"24h"`, `"1h30m"` etc

---

## [ha]

High availability / failover settings. this is a **bootstrap section** — it lives in the TOML config file, not the database. each node needs its own `[ha]` block. see [high-availability.md](high-availability.md) for full setup guide

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable HA |
| `role` | string | required | `"primary"` or `"secondary"` |
| `peer_address` | string | required | Address of the other node e.g. `"192.168.1.2:8067"` |
| `listen_address` | string | required | Address to listen for peer connections e.g. `"0.0.0.0:8067"` |
| `heartbeat_interval` | duration | `"1s"` | How often to send heartbeats |
| `failover_timeout` | duration | `"10s"` | How long before declaring peer dead |
| `sync_batch_size` | int | `100` | Leases per batch during bulk sync |

### [ha.tls]

Optional TLS for peer communication. recommended if peers are on untrusted networks

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable TLS |
| `cert_file` | string | | Path to TLS certificate |
| `key_file` | string | | Path to TLS private key |
| `ca_file` | string | | Path to CA certificate for peer verification |

```toml
[ha]
enabled = true
role = "primary"
peer_address = "192.168.1.2:8067"
listen_address = "0.0.0.0:8067"
heartbeat_interval = "1s"
failover_timeout = "10s"

  [ha.tls]
  enabled = true
  cert_file = "/etc/athena-dhcpd/tls/server.crt"
  key_file = "/etc/athena-dhcpd/tls/server.key"
  ca_file = "/etc/athena-dhcpd/tls/ca.crt"
```

---

## [hooks]

Event hooks — run scripts or call webhooks when things happen. see [event-hooks.md](event-hooks.md) for details

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `event_buffer_size` | int | `10000` | Event bus buffer size. if this fills up events get dropped (with a log warning) |
| `script_concurrency` | int | `4` | Max concurrent script executions |
| `script_timeout` | duration | `"10s"` | Default script timeout |

### [[hooks.script]]

Script hooks. can have multiple

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Hook name (for logging) |
| `events` | string[] | Event types to trigger on. supports wildcards like `"lease.*"` or `"*"` |
| `command` | string | Shell command to execute |
| `timeout` | duration | Override default timeout |
| `subnets` | string[] | Optional subnet filter — only fire for events from these subnets |

### [[hooks.webhook]]

Webhook hooks. can have multiple

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Hook name |
| `events` | string[] | Event types to trigger on |
| `url` | string | Webhook URL |
| `method` | string | HTTP method (default `"POST"`) |
| `headers` | map | Extra HTTP headers |
| `timeout` | duration | HTTP request timeout |
| `retries` | int | Max retry attempts (default 3) |
| `retry_backoff` | duration | Backoff between retries (default `"2s"`, doubles each retry) |
| `secret` | string | HMAC-SHA256 secret. if set, requests get an `X-Athena-Signature` header |
| `template` | string | `"slack"`, `"teams"`, or empty for raw JSON |

```toml
[hooks]
event_buffer_size = 1024
script_concurrency = 4
script_timeout = "30s"

  [[hooks.script]]
  name = "on-lease"
  events = ["lease.ack", "lease.release", "lease.expire"]
  command = "/usr/local/bin/athena-hook.sh"
  timeout = "10s"
  subnets = ["192.168.1.0/24"]

  [[hooks.webhook]]
  name = "slack-alerts"
  events = ["conflict.detected", "conflict.permanent", "ha.failover"]
  url = "https://hooks.slack.com/services/T00/B00/XXXXX"
  template = "slack"
  retries = 3
  secret = "my-hmac-secret"
```

---

## [ddns]

Dynamic DNS. built-in, not a script hook. see [dynamic-dns.md](dynamic-dns.md) for setup guides

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable DDNS |
| `allow_client_fqdn` | bool | `false` | Honour client-supplied FQDN (option 81) |
| `fallback_to_mac` | bool | `false` | Use MAC-based hostname if no hostname/FQDN available |
| `ttl` | int | `300` | DNS record TTL in seconds |
| `update_on_renew` | bool | `false` | Also update DNS on lease renewals (not just initial ACK) |
| `conflict_policy` | string | `"overwrite"` | What to do when a DNS record already exists |
| `use_dhcid` | bool | `false` | Create DHCID records for conflict detection (RFC 4701) |

### [ddns.forward] and [ddns.reverse]

Zone configuration. both have the same fields

| Field | Type | Description |
|-------|------|-------------|
| `zone` | string | DNS zone name (with trailing dot) e.g. `"example.com."` |
| `method` | string | `"rfc2136"`, `"powerdns_api"`, or `"technitium_api"` |
| `server` | string | DNS server address. for rfc2136: `"ns1:53"`. for APIs: `"http://dns-api:8081"` |
| `tsig_name` | string | TSIG key name (rfc2136 only) |
| `tsig_algorithm` | string | TSIG algorithm (rfc2136 only). e.g. `"hmac-sha256"` |
| `tsig_secret` | string | TSIG secret, base64 encoded (rfc2136 only) |
| `api_key` | string | API key (powerdns_api and technitium_api only) |

### [[ddns.zone_override]]

Per-subnet zone overrides. useful if different subnets live in different DNS zones

| Field | Type | Description |
|-------|------|-------------|
| `subnet` | string | Subnet CIDR to match |
| `forward_zone` | string | Override forward zone |
| `reverse_zone` | string | Override reverse zone |
| `method` | string | Override update method |
| `server` | string | Override DNS server |
| `api_key` | string | Override API key |
| `tsig_name` | string | Override TSIG key name |
| `tsig_algorithm` | string | Override TSIG algorithm |
| `tsig_secret` | string | Override TSIG secret |

```toml
[ddns]
enabled = true
allow_client_fqdn = true
ttl = 300
use_dhcid = true

  [ddns.forward]
  zone = "example.com."
  method = "rfc2136"
  server = "ns1.example.com:53"
  tsig_name = "dhcp-update."
  tsig_algorithm = "hmac-sha256"
  tsig_secret = "base64-encoded-secret"

  [ddns.reverse]
  zone = "1.168.192.in-addr.arpa."
  method = "rfc2136"
  server = "ns1.example.com:53"
  tsig_name = "dhcp-update."
  tsig_algorithm = "hmac-sha256"
  tsig_secret = "base64-encoded-secret"

  [[ddns.zone_override]]
  subnet = "10.0.0.0/24"
  forward_zone = "lab.example.com."
  reverse_zone = "0.0.10.in-addr.arpa."
  method = "rfc2136"
  server = "ns2.example.com:53"
```

---

## [dns]

Built-in DNS proxy. see [dns-proxy.md](dns-proxy.md) for the full deep-dive including filter lists, DoH, zone overrides, and lease registration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the DNS proxy |
| `listen_udp` | string | `"0.0.0.0:53"` | UDP listen address. TCP automatically listens on same port |
| `listen_doh` | string | | DNS-over-HTTPS listen address. empty = disabled |
| `domain` | string | | Local domain for DHCP lease registrations |
| `ttl` | int | `60` | TTL in seconds for local zone records |
| `register_leases` | bool | `false` | Auto-create A records from DHCP leases |
| `register_leases_ptr` | bool | `false` | Also create PTR records for reverse lookups |
| `forwarders` | string[] | | Upstream DNS servers |
| `use_root_servers` | bool | `false` | Use root servers instead of forwarders |
| `cache_size` | int | `10000` | Max cached responses |
| `cache_ttl` | duration | `"5m"` | How long to cache upstream responses |

### [dns.doh_tls]

| Field | Type | Description |
|-------|------|-------------|
| `cert_file` | string | TLS certificate for DoH |
| `key_file` | string | TLS private key for DoH |

### [[dns.record]]

Static DNS records

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Record name e.g. `"nas.home.lan"` |
| `type` | string | `A`, `AAAA`, `CNAME`, `PTR`, `TXT`, `MX`, `SRV` |
| `value` | string | Record value |
| `ttl` | int | Optional TTL override |

### [[dns.zone_override]]

Route queries for specific domains to specific nameservers

| Field | Type | Description |
|-------|------|-------------|
| `zone` | string | Domain to match |
| `nameserver` | string | DNS server address |
| `doh` | bool | Use DoH to reach this nameserver |
| `doh_url` | string | DoH URL |

### [[dns.list]]

Dynamic filter lists (blocklists/allowlists) for domain blocking

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | List name |
| `url` | string | required | Download URL |
| `type` | string | `"block"` | `"block"` or `"allow"` |
| `format` | string | `"hosts"` | `"hosts"`, `"domains"`, or `"adblock"` |
| `action` | string | `"nxdomain"` | `"nxdomain"`, `"zero"`, or `"refuse"` |
| `enabled` | bool | `true` | Enable/disable without removing |
| `refresh_interval` | duration | `"24h"` | Re-download interval (min 1m) |

```toml
[dns]
enabled = true
listen_udp = "0.0.0.0:53"
domain = "home.lan"
ttl = 60
register_leases = true
register_leases_ptr = true
forwarders = ["1.1.1.1", "8.8.8.8"]
cache_size = 10000
cache_ttl = "5m"

  [[dns.record]]
  name = "nas.home.lan"
  type = "A"
  value = "10.0.0.50"

  [[dns.zone_override]]
  zone = "corp.internal"
  nameserver = "10.0.0.2"

  [[dns.list]]
  name = "steven-black"
  url = "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  type = "block"
  format = "hosts"
  action = "nxdomain"
  enabled = true
  refresh_interval = "24h"
```

---

## [defaults]

Global default options applied to all subnets unless overridden

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `lease_time` | duration | `"12h"` | Default lease duration |
| `renewal_time` | duration | `"6h"` | T1 — when clients should try to renew |
| `rebind_time` | duration | `"10h30m"` | T2 — when clients should try to rebind |
| `dns_servers` | string[] | | Default DNS servers |
| `domain_name` | string | | Default domain name |

```toml
[defaults]
lease_time = "8h"
renewal_time = "4h"
rebind_time = "7h"
dns_servers = ["8.8.8.8", "8.8.4.4"]
domain_name = "example.com"
```

---

## [[subnet]]

Subnet definitions. you need at least one or the server has nothing to do

| Field | Type | Description |
|-------|------|-------------|
| `network` | string | Subnet in CIDR notation e.g. `"192.168.1.0/24"` |
| `interface` | string | Network interface for this subnet (e.g. `"eth0"`, `"vlan100"`). the server listens on each unique interface |
| `routers` | string[] | Default gateway(s) — option 3 |
| `dns_servers` | string[] | DNS servers for this subnet — option 6. overrides `[defaults]` |
| `domain_name` | string | Domain name for this subnet — option 15 |
| `lease_time` | duration | Lease time override for this subnet |
| `renewal_time` | duration | T1 override |
| `rebind_time` | duration | T2 override |
| `ntp_servers` | string[] | NTP servers — option 42 |

### [[subnet.pool]]

IP address pools within a subnet. you can have multiple pools with different match criteria for fancy stuff like assigning different ranges based on where the client is plugged in

| Field | Type | Description |
|-------|------|-------------|
| `range_start` | string | First IP in the pool |
| `range_end` | string | Last IP in the pool |
| `lease_time` | duration | Pool-specific lease time override |
| `match_circuit_id` | string | Only serve this pool if relay circuit ID matches (glob pattern) |
| `match_remote_id` | string | Only serve this pool if relay remote ID matches (glob pattern) |
| `match_vendor_class` | string | Only serve this pool if vendor class (option 60) matches (glob pattern) |
| `match_user_class` | string | Only serve this pool if user class (option 77) matches (glob pattern) |

Pool matching uses glob patterns so you can do things like `"eth0/1/*"` to match any port on a specific switch. if a pool has match criteria, it only serves clients that match. pools without match criteria act as the default fallback

### [[subnet.reservation]]

Static IP reservations. these bypass the pool allocator entirely

| Field | Type | Description |
|-------|------|-------------|
| `mac` | string | MAC address to match (one of `mac` or `identifier` required) |
| `identifier` | string | Client identifier to match (alternative to MAC) |
| `ip` | string | IP to assign |
| `hostname` | string | Hostname for option 12 |
| `dns_servers` | string[] | Per-reservation DNS server override |
| `ddns_hostname` | string | Override FQDN for DDNS registration |

### [[subnet.option]]

Custom DHCP options. for anything not covered by the built-in fields

| Field | Type | Description |
|-------|------|-------------|
| `code` | int | DHCP option code (1-254) |
| `type` | string | Data type: `"ip"`, `"ip_list"`, `"string"`, `"uint8"`, `"uint16"`, `"uint32"`, `"bool"`, `"bytes"` |
| `value` | varies | The option value. type depends on `type` field |

```toml
[[subnet]]
network = "192.168.1.0/24"
interface = "eth0"
routers = ["192.168.1.1"]
dns_servers = ["192.168.1.1", "8.8.8.8"]
domain_name = "office.example.com"
lease_time = "12h"
ntp_servers = ["192.168.1.1"]

  [[subnet.pool]]
  range_start = "192.168.1.100"
  range_end = "192.168.1.200"

  [[subnet.pool]]
  range_start = "192.168.1.201"
  range_end = "192.168.1.250"
  match_circuit_id = "eth0/1/*"
  lease_time = "4h"

  [[subnet.reservation]]
  mac = "00:11:22:33:44:55"
  ip = "192.168.1.10"
  hostname = "printer"
  ddns_hostname = "printer.office.example.com"

  [[subnet.option]]
  code = 66
  type = "string"
  value = "tftp.example.com"
```

---

## [api]

HTTP API and web UI settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the API server |
| `listen` | string | `"0.0.0.0:8067"` | Listen address |
| `web_ui` | bool | `false` | Enable the embedded React web UI |

### [api.auth]

Authentication config. if both `auth_token` and `users` are empty, auth is disabled (everything is admin). probably dont do that in production

| Field | Type | Description |
|-------|------|-------------|
| `auth_token` | string | Bearer token for API access |

### [[api.auth.users]]

Web UI user accounts. passwords MUST be bcrypt hashes

| Field | Type | Description |
|-------|------|-------------|
| `username` | string | Login username |
| `password_hash` | string | bcrypt hash of the password |
| `role` | string | `"admin"` (read/write) or `"viewer"` (read-only) |

generate a password hash:
```bash
htpasswd -nbBC 10 "" 'yourpassword' | cut -d: -f2
```

### [api.tls]

HTTPS support for the API

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable TLS |
| `cert_file` | string | | Certificate file path |
| `key_file` | string | | Private key file path |

### [api.session]

Session cookie settings for the web UI

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `cookie_name` | string | `"athena_session"` | Session cookie name |
| `expiry` | duration | `"24h"` | Session lifetime |
| `secure` | bool | `false` | Set Secure flag on cookie. enable this when using TLS |

```toml
[api]
enabled = true
listen = "127.0.0.1:8080"
web_ui = true

  [api.auth]
  auth_token = "my-secret-api-token"

    [[api.auth.users]]
    username = "admin"
    password_hash = "$2y$10$..."
    role = "admin"

    [[api.auth.users]]
    username = "readonly"
    password_hash = "$2y$10$..."
    role = "viewer"

  [api.tls]
  enabled = true
  cert_file = "/etc/athena-dhcpd/tls/api.crt"
  key_file = "/etc/athena-dhcpd/tls/api.key"

  [api.session]
  expiry = "24h"
  secure = true
```

---

## Config Validation

the config parser validates a bunch of stuff at load time so you dont find out about typos at 3am:

- overlapping subnets (two subnets covering the same IP space)
- overlapping pool ranges within a subnet
- pool range ordering (start must be <= end, yes this has happened)
- required fields (server_id, at least one subnet, etc)
- sane defaults for anything you don't specify

if validation fails, the server won't start. if validation fails during a SIGHUP reload, it keeps the old config and logs the error
