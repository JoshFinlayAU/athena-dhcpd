# DNS Proxy

athena-dhcpd has a built-in DNS proxy so you dont need to run a separate DNS server. it handles local name resolution for DHCP clients, caches upstream responses, supports DNS-over-HTTPS, and can block ads/malware/whatever via filter lists

if you just want your DHCP clients to resolve each other by hostname — enable the DNS proxy, point your clients at it, done

## how it works

the query pipeline runs in this order:

1. **filter lists** — is this domain on a blocklist? block it. on an allowlist? let it through regardless
2. **local zone** — do we have a record for this? (static records + DHCP lease registrations)
3. **cache** — have we seen this query recently? return cached response
4. **zone overrides** — does this domain match an override? forward to that specific nameserver
5. **upstream forwarders** — send it to your configured forwarders (1.1.1.1, 8.8.8.8, etc)

every step is skipped if it doesn't match, falling through to the next one

## basic setup

```toml
[dns]
enabled = true
listen_udp = "0.0.0.0:53"
domain = "example.com"
ttl = 60
register_leases = true
register_leases_ptr = true
forwarders = ["1.1.1.1", "8.8.8.8"]
cache_size = 10000
cache_ttl = "5m"
```

then point your DHCP clients at the server:
```toml
[defaults]
dns_servers = ["10.0.0.1"]  # your athena-dhcpd server IP
domain_name = "example.com"
```

thats it. clients get IPs via DHCP, get told to use this server for DNS, and can resolve each other by hostname

## configuration reference

### [dns]

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the DNS proxy |
| `listen_udp` | string | `"0.0.0.0:53"` | UDP listen address. TCP automatically listens on same port |
| `listen_doh` | string | | DNS-over-HTTPS listen address e.g. `"0.0.0.0:443"`. empty = disabled |
| `domain` | string | | Local domain for DHCP lease registrations e.g. `"home.lan"` |
| `ttl` | int | `60` | TTL in seconds for local zone records |
| `register_leases` | bool | `false` | Auto-create A records from DHCP leases |
| `register_leases_ptr` | bool | `false` | Also create PTR records for reverse lookups |
| `forwarders` | string[] | | Upstream DNS servers e.g. `["1.1.1.1", "8.8.8.8"]` |
| `use_root_servers` | bool | `false` | Use root servers instead of forwarders (recursive mode) |
| `cache_size` | int | `10000` | Max cached responses |
| `cache_ttl` | duration | `"5m"` | How long to cache responses |

### [dns.doh_tls]

TLS config for DNS-over-HTTPS. only needed if `listen_doh` is set

| Field | Type | Description |
|-------|------|-------------|
| `cert_file` | string | Path to TLS certificate |
| `key_file` | string | Path to TLS private key |

if no TLS config is provided and DoH is enabled, it runs plain HTTP (useful behind a reverse proxy)

### [[dns.record]]

Static DNS records. for stuff that isn't a DHCP client

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Record name e.g. `"nas.home.lan"` |
| `type` | string | Record type: `A`, `AAAA`, `CNAME`, `PTR`, `TXT`, `MX`, `SRV` |
| `value` | string | Record value |
| `ttl` | int | Optional TTL override |

```toml
[[dns.record]]
name = "nas.home.lan"
type = "A"
value = "10.0.0.50"

[[dns.record]]
name = "mail.home.lan"
type = "MX"
value = "10 smtp.home.lan"

[[dns.record]]
name = "vpn.home.lan"
type = "CNAME"
value = "gateway.home.lan."

[[dns.record]]
name = "_http._tcp.home.lan"
type = "SRV"
value = "10 0 8080 webserver.home.lan"
```

### [[dns.zone_override]]

Route queries for specific domains to specific nameservers. useful for split-horizon DNS, internal corporate zones, or forwarding `.local` somewhere specific

| Field | Type | Description |
|-------|------|-------------|
| `zone` | string | Domain to match (most specific match wins) |
| `nameserver` | string | DNS server address e.g. `"10.0.0.2:53"` |
| `doh` | bool | Use DNS-over-HTTPS to reach this nameserver |
| `doh_url` | string | DoH URL e.g. `"https://dns.example.com/dns-query"` |

```toml
[[dns.zone_override]]
zone = "corp.example.com"
nameserver = "10.0.0.2"

[[dns.zone_override]]
zone = "internal.dev"
doh = true
doh_url = "https://internal-dns.dev/dns-query"
```

the override matching walks up the domain labels. a query for `host.corp.example.com` matches the `corp.example.com` override. most specific match wins

### [[dns.list]]

Dynamic filter lists for blocking domains. see [filter lists](#filter-lists) below

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | List name (for display and API) |
| `url` | string | required | URL to download the list from |
| `type` | string | `"block"` | `"block"` or `"allow"` |
| `format` | string | `"hosts"` | `"hosts"`, `"domains"`, or `"adblock"` |
| `action` | string | `"nxdomain"` | What to return for blocked queries: `"nxdomain"`, `"zero"`, `"refuse"` |
| `enabled` | bool | `true` | Enable/disable without removing config |
| `refresh_interval` | duration | `"24h"` | How often to re-download. minimum 1 minute |

---

## DHCP lease registration

when `register_leases = true`, the DNS proxy subscribes to the DHCP event bus. on every lease ACK or renewal, it creates an A record:

```
hostname.domain. → lease IP
```

if `register_leases_ptr = true`, it also creates a PTR record for reverse lookups:

```
x.x.x.x.in-addr.arpa. → hostname.domain.
```

on lease release or expiry, the records are removed

the hostname comes from the DHCP client (option 12). if a reservation has a `hostname` field, that takes priority. clients without a hostname don't get DNS records

---

## DNS-over-HTTPS

the proxy supports RFC 8484 DNS-over-HTTPS, both as a server (accepting DoH queries) and as a client (forwarding to DoH upstreams via zone overrides)

### as a server

```toml
[dns]
listen_doh = "0.0.0.0:443"

  [dns.doh_tls]
  cert_file = "/etc/athena-dhcpd/tls/dns.crt"
  key_file = "/etc/athena-dhcpd/tls/dns.key"
```

supports both GET (`?dns=` base64url parameter) and POST (`application/dns-message` body) methods per the RFC

### as a client (zone overrides)

```toml
[[dns.zone_override]]
zone = "secure.example.com"
doh = true
doh_url = "https://dns.cloudflare.com/dns-query"
```

---

## caching

responses from upstream forwarders are cached in memory. the cache:

- stores up to `cache_size` entries (default 10000, LRU eviction)
- respects `cache_ttl` as the max cache time
- returns copies so cached responses cant be mutated
- can be flushed manually via the API or web UI
- does NOT cache local zone or filter list responses (no point)

---

## filter lists

the DNS proxy can download and apply domain blocklists and allowlists. this is basically a built-in pihole

### list types

- **block** — queries matching this list are blocked (returns NXDOMAIN, 0.0.0.0, or REFUSED depending on `action`)
- **allow** — queries matching this list are always allowed, even if they appear on a blocklist. allowlists are checked first

### list formats

three formats are supported:

**hosts** — standard hosts file format (like Steven Black's list, Pi-hole lists)
```
0.0.0.0 ads.example.com
127.0.0.1 tracker.example.com
# comments are ignored
```

**domains** — one domain per line
```
ads.example.com
tracker.example.com
```

**adblock** — adblock filter syntax (only `||domain^` rules, not full adblock)
```
||ads.example.com^
||tracker.example.com^
! comments start with !
```

### block actions

| Action | Behaviour |
|--------|-----------|
| `nxdomain` | Return NXDOMAIN (domain does not exist). most compatible, default |
| `zero` | Return 0.0.0.0 for A queries, :: for AAAA. some apps handle this better than NXDOMAIN |
| `refuse` | Return REFUSED. aggressive, some clients retry forever |

### subdomain matching

blocking `ads.example.com` also blocks `sub.ads.example.com`, `deep.sub.ads.example.com`, etc. the check walks up the domain labels

### allowlist priority

allowlists are ALWAYS checked before blocklists. if a domain appears on both an allowlist and a blocklist, it is allowed. this lets you subscribe to aggressive blocklists and then carve out exceptions

### refresh

lists are downloaded on startup and then refreshed at the configured `refresh_interval`. minimum interval is 1 minute (to not hammer list providers). default is 24 hours

you can also trigger a manual refresh via the API or web UI at any time

### example config

```toml
# block ads and trackers
[[dns.list]]
name = "steven-black-hosts"
url = "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
type = "block"
format = "hosts"
action = "nxdomain"
enabled = true
refresh_interval = "24h"

# block malware/threats
[[dns.list]]
name = "hagezi-threat"
url = "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/domains/multi.txt"
type = "block"
format = "domains"
action = "nxdomain"
enabled = true
refresh_interval = "12h"

# but allow some stuff that blocklists are too aggressive about
[[dns.list]]
name = "my-allowlist"
url = "https://example.com/my-allowlist.txt"
type = "allow"
format = "domains"
enabled = true
refresh_interval = "6h"
```

### popular blocklists

some good ones to start with:

| List | URL | Format |
|------|-----|--------|
| Steven Black unified | `https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts` | hosts |
| Hagezi multi | `https://raw.githubusercontent.com/hagezi/dns-blocklists/main/domains/multi.txt` | domains |
| OISD small | `https://small.oisd.nl/domainswild` | domains |
| AdGuard DNS | `https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt` | adblock |
| Energized basic | `https://energized.pro/basic/formats/hosts` | hosts |

---

## API endpoints

all DNS endpoints require authentication. admin-only endpoints are noted

### GET /api/v1/dns/stats

returns DNS proxy statistics

```json
{
  "zone_records": 42,
  "cache_entries": 1337,
  "forwarders": 2,
  "overrides": 1,
  "domain": "home.lan",
  "filter_lists": 2,
  "blocked_domains": 150000
}
```

### GET /api/v1/dns/records

returns all records in the local zone (static + lease-registered)

```json
{
  "records": [
    {"name": "nas.home.lan.", "type": "A", "value": "10.0.0.50", "ttl": 60},
    {"name": "printer.home.lan.", "type": "A", "value": "10.0.0.20", "ttl": 60}
  ],
  "count": 2
}
```

### POST /api/v1/dns/cache/flush *(admin)*

flushes the DNS response cache

```json
{"status": "flushed"}
```

### GET /api/v1/dns/lists

returns status of all filter lists including domain counts, last refresh time, errors

```json
{
  "lists": [
    {
      "name": "steven-black-hosts",
      "url": "https://...",
      "type": "block",
      "format": "hosts",
      "action": "nxdomain",
      "enabled": true,
      "domain_count": 150000,
      "last_refresh": "2026-02-26T08:00:00Z",
      "next_refresh": "2026-02-27T08:00:00Z",
      "last_error": "",
      "refresh_interval": "24h"
    }
  ],
  "total_domains": 150000
}
```

### POST /api/v1/dns/lists/refresh *(admin)*

triggers manual refresh. send empty body for all lists, or specify one:

```bash
# refresh all lists
curl -X POST http://localhost:8067/api/v1/dns/lists/refresh

# refresh a specific list
curl -X POST http://localhost:8067/api/v1/dns/lists/refresh \
  -H "Content-Type: application/json" \
  -d '{"name": "steven-black-hosts"}'
```

### POST /api/v1/dns/lists/test

test a domain against all active filter lists

```bash
curl -X POST http://localhost:8067/api/v1/dns/lists/test \
  -H "Content-Type: application/json" \
  -d '{"domain": "ads.example.com"}'
```

```json
{
  "domain": "ads.example.com",
  "blocked": true,
  "action": "nxdomain",
  "list": "steven-black-hosts",
  "matches": [
    {"list": "steven-black-hosts", "type": "block"}
  ]
}
```

---

## web UI

the DNS Filtering page in the web UI gives you:

- **stats dashboard** — blocked domain count, filter list count, cache size, zone records
- **domain tester** — type a domain and instantly see if it would be blocked and by which list
- **list management** — view all lists with domain counts, last refresh, errors. refresh individual lists or all at once
- **cache flush** — one click to clear the DNS cache

![DNS Filtering](../screenshots/dns_filtering.png)

the DNS Query Log page shows live streaming queries with source device identification from DHCP leases:

![DNS Query Log](../screenshots/dns_query_log.png)

filter list configuration (add/remove/edit lists) is in the Config page under the DNS section

![Config — DNS Proxy](../screenshots/config_dns_proxy.png)

---

## protocols

the DNS proxy listens on:

- **UDP** (always) — standard DNS on configured port, default 53
- **TCP** (always) — automatically on same port as UDP. handles large responses that don't fit in UDP
- **DoH** (optional) — DNS-over-HTTPS on a separate port if `listen_doh` is configured

all three protocols go through the same query pipeline so behaviour is identical regardless of transport

---

## high availability

if you're running two athena-dhcpd nodes in HA mode, you need a floating IP so DNS clients always have a stable address to query. athena-dhcpd has built-in floating VIP management — configure VIPs in the web UI or API and the active node holds them automatically. see [Floating Virtual IPs](ha-floating-ip.md) for the full setup guide

## practical tips

- **point DNS at yourself** — set `dns_servers` in your subnet/defaults config to the server's own IP (or the floating VIP if running HA). clients will use the built-in proxy automatically
- **start with one blocklist** — steven black unified is a good default. add more later if you want more aggressive blocking
- **use allowlists for exceptions** — when a blocklist is too aggressive, don't remove the whole list. add an allowlist with the domains you need
- **cache size** — 10000 is fine for most home/office networks. bump it up for larger deployments
- **zone overrides for split DNS** — if you have internal domains that resolve differently inside vs outside your network, use zone overrides to point them at the right nameserver
- **DoH behind reverse proxy** — if you already have nginx/caddy handling TLS, set `listen_doh` to a local port and proxy to it. skip the `doh_tls` config entirely
