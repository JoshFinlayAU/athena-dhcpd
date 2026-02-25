# Dynamic DNS

DDNS in athena-dhcpd is a first-class feature, not some script hook afterthought. when a client gets a lease, the server automatically registers A and PTR records. when the lease expires or gets released, it cleans them up (best effort — DNS is like that)

## how it works

the DDNS manager subscribes to the event bus and watches for:

- `lease.ack` → create/update A record (forward) + PTR record (reverse)
- `lease.renew` → optionally update records (if `update_on_renew = true`)
- `lease.release` / `lease.expire` → remove A + PTR records

DNS updates are **always async** — they never block a DHCP response. your client gets their IP immediately, DNS catches up in the background. if a DNS update fails, it retries with exponential backoff (3 attempts, starting at 5s)

## FQDN construction

the server builds the FQDN for each lease using this priority:

1. **Client FQDN** (option 81) — if `allow_client_fqdn = true` and the client sends one
2. **Hostname + domain** — hostname from option 12, domain from the forward zone config
3. **MAC fallback** — if `fallback_to_mac = true`, generates a hostname from the MAC like `mac-aabbccddeeff`
4. **Skip** — if none of the above produce a usable name, no DNS update happens

hostnames get sanitized: only alphanumeric chars and hyphens allowed, converted to lowercase, max 63 chars per label

## supported DNS backends

### RFC 2136 (BIND, Knot, Windows DNS, CoreDNS)

the standard dynamic update protocol. works with basically any DNS server that supports it

```toml
[ddns.forward]
zone = "example.com."
method = "rfc2136"
server = "ns1.example.com:53"
tsig_name = "dhcp-update."
tsig_algorithm = "hmac-sha256"
tsig_secret = "base64-encoded-tsig-secret"
```

TSIG signing is used for authentication. you need to set up a TSIG key on your DNS server and share the secret. supported algorithms: `hmac-md5`, `hmac-sha1`, `hmac-sha256`, `hmac-sha512`

#### BIND example

in your BIND config:
```
key "dhcp-update" {
    algorithm hmac-sha256;
    secret "your-base64-secret-here";
};

zone "example.com" {
    type master;
    file "example.com.zone";
    allow-update { key "dhcp-update"; };
};
```

#### Knot DNS example
```yaml
key:
  - id: dhcp-update
    algorithm: hmac-sha256
    secret: your-base64-secret

acl:
  - id: dhcp-update-acl
    key: dhcp-update
    action: update

zone:
  - domain: example.com
    acl: dhcp-update-acl
```

### PowerDNS API

uses the PowerDNS HTTP API directly. no TSIG needed, authenticates with an API key

```toml
[ddns.forward]
zone = "example.com."
method = "powerdns_api"
server = "http://pdns-server:8081"
api_key = "your-pdns-api-key"
```

make sure the PowerDNS API is enabled:
```
api=yes
api-key=your-pdns-api-key
webserver=yes
webserver-address=0.0.0.0
webserver-port=8081
```

### Technitium API

uses the Technitium DNS Server HTTP API

```toml
[ddns.forward]
zone = "example.com."
method = "technitium_api"
server = "http://technitium:5380"
api_key = "your-technitium-api-token"
```

## forward and reverse zones

most setups want both:

- **Forward zone** — A records mapping `hostname.example.com → 192.168.1.100`
- **Reverse zone** — PTR records mapping `100.1.168.192.in-addr.arpa → hostname.example.com`

reverse zone is optional. if you don't configure `[ddns.reverse]`, only forward records get created

```toml
[ddns.forward]
zone = "example.com."
method = "rfc2136"
server = "ns1.example.com:53"
tsig_name = "dhcp-update."
tsig_algorithm = "hmac-sha256"
tsig_secret = "forward-zone-secret"

[ddns.reverse]
zone = "1.168.192.in-addr.arpa."
method = "rfc2136"
server = "ns1.example.com:53"
tsig_name = "dhcp-update."
tsig_algorithm = "hmac-sha256"
tsig_secret = "reverse-zone-secret"
```

forward and reverse zones can use different servers, methods, and credentials. useful if your forward zone is in PowerDNS but reverse is in BIND or whatever

## per-subnet zone overrides

if you have different subnets in different DNS zones (common in larger networks):

```toml
[[ddns.zone_override]]
subnet = "10.0.0.0/24"
forward_zone = "lab.example.com."
reverse_zone = "0.0.10.in-addr.arpa."
method = "rfc2136"
server = "ns2.example.com:53"
tsig_name = "lab-update."
tsig_algorithm = "hmac-sha256"
tsig_secret = "lab-zone-secret"
```

any field you specify in the override replaces the default. fields you leave out fall back to the main `[ddns.forward]` / `[ddns.reverse]` config

## DHCID records

RFC 4701 defines DHCID records for DNS conflict detection. when `use_dhcid = true`, the server creates a DHCID record alongside the A record. this lets multiple DHCP servers coordinate without stepping on each others DNS entries

mostly useful in HA setups where both nodes might try to update DNS for the same client

## cleanup

when a lease expires or gets released:
- A record removal is attempted (best-effort)
- PTR record removal is attempted (best-effort)
- if removal fails, it logs a warning and moves on. DNS records will eventually become stale, but that's better than crashing or blocking

## security notes

- TSIG secrets and API keys are **never logged** — they're redacted in all log output
- the `/api/v1/config` endpoint redacts secrets as `"***REDACTED***"`
- secrets should have restrictive file permissions on the config file (0600)
- the systemd service file sets up `ProtectSystem=strict` which helps

## metrics

- `athena_dhcpd_ddns_updates_total{type,result}` — counts by operation type (add_a, add_ptr, remove_a, remove_ptr) and result (success, error)
- `athena_dhcpd_ddns_update_duration_seconds{type}` — latency histogram by operation type
