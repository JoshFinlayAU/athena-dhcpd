# Floating Virtual IPs

athena-dhcpd has built-in floating VIP management. no keepalived, no external tools, no scripts. configure your virtual IPs in the web UI or API and the active HA node holds them automatically. on failover, the new active node acquires the VIPs and sends gratuitous ARPs to update switch MAC tables

## the problem

say you have two nodes:
- **node A** (primary): `10.0.0.1`
- **node B** (secondary): `10.0.0.2`

both run athena-dhcpd with HA and the DNS proxy enabled. you hand out `dns_servers = ["10.0.0.1"]` to clients via DHCP

node A crashes. node B takes over DHCP — great, new clients get IPs. but every existing client still has `10.0.0.1` as their DNS server. DNS is broken for them until node A comes back or they renew their lease and get node B's address

thats not good enough

## the solution: built-in floating VIPs

use a third IP address — the VIP — that floats between the two nodes. clients always use the VIP for DNS. athena-dhcpd manages the VIP itself based on the HA state machine

### network layout

| Host | Physical IP | Role |
|------|------------|------|
| node A | `10.0.0.1` | primary |
| node B | `10.0.0.2` | secondary |
| VIP | `10.0.0.3` | floating (active node owns this) |

## setup

### 1. configure HA

standard HA config on both nodes. peer addresses use the real IPs, not the VIP:

```toml
# node A
[ha]
enabled = true
role = "primary"
peer_address = "10.0.0.2:8068"
listen_address = "0.0.0.0:8068"
```

```toml
# node B
[ha]
enabled = true
role = "secondary"
peer_address = "10.0.0.1:8068"
listen_address = "0.0.0.0:8068"
```

### 2. add floating VIPs

VIPs are stored in the database, not the TOML file. configure them via:

**web UI** — go to Configuration > High Availability, scroll down to "Floating Virtual IPs", click "Add VIP"

**setup wizard** — the HA config step includes a VIP section

**API** — `PUT /api/v2/vips`:

```bash
curl -X PUT http://localhost:8067/api/v2/vips \
  -H "Authorization: Bearer mytoken" \
  -H "Content-Type: application/json" \
  -d '[
    {"ip": "10.0.0.3", "cidr": 24, "interface": "eth0", "label": "DNS VIP"}
  ]'
```

each VIP entry has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ip` | string | yes | Virtual IP address |
| `cidr` | int | yes | Prefix length (1-32) |
| `interface` | string | yes | Network interface to add the IP to |
| `label` | string | no | Human-readable label (shown in web UI) |

### multiple VIPs

you can configure multiple VIPs — useful when you need a VIP per VLAN so each subnet gets its own DNS server address:

```bash
curl -X PUT http://localhost:8067/api/v2/vips \
  -H "Authorization: Bearer mytoken" \
  -H "Content-Type: application/json" \
  -d '[
    {"ip": "10.0.0.3", "cidr": 24, "interface": "eth0", "label": "Management VIP"},
    {"ip": "10.1.0.3", "cidr": 24, "interface": "eth0.10", "label": "VLAN 10 DNS"},
    {"ip": "10.2.0.3", "cidr": 24, "interface": "eth0.20", "label": "VLAN 20 DNS"}
  ]'
```

### 3. configure DNS proxy

make sure the DNS proxy binds to `0.0.0.0` (the default) so it responds on the VIP. set this in **Configuration > DNS Proxy** or via `PUT /api/v2/config/dns`:

```json
{
  "enabled": true,
  "listen_udp": "0.0.0.0:53"
}
```

if you bind to a specific IP, it wont answer queries on the VIP when it floats over

### 4. hand out the VIP as DNS server

tell DHCP clients to use the floating IP for DNS. set this in **Configuration > Defaults** or per-subnet in **Configuration > Subnets**:

```json
{
  "dns_servers": ["10.0.0.3"],
  "domain_name": "home.lan"
}
```

or per-subnet (in the subnet config):
```json
[
  {"network": "10.0.0.0/24", "dns_servers": ["10.0.0.3"]},
  {"network": "10.1.0.0/24", "dns_servers": ["10.1.0.3"]}
]
```

## how it works

the VIP manager is integrated directly into the HA state machine. no health check scripts, no VRRP protocol, no external dependencies

### normal operation
1. node A is ACTIVE, node B is STANDBY
2. node A holds all configured VIPs (added via `ip addr add`)
3. gratuitous ARPs sent on acquire so switches learn the MAC immediately
4. clients send DNS queries to `10.0.0.3` → node A answers
5. leases, conflict table, and VIP config all continuously synced to node B

### failover
1. node A crashes
2. node B detects peer down after `failover_timeout` (default 10s)
3. node B transitions to ACTIVE
4. VIP manager acquires all VIPs on node B (`ip addr add`)
5. gratuitous ARP sent for each VIP → switches update MAC tables
6. DNS queries to `10.0.0.3` now reach node B
7. total DNS downtime: roughly equal to `failover_timeout`

### recovery
1. node A comes back online
2. peers reconnect, bulk sync happens
3. node A transitions back to ACTIVE, node B to STANDBY
4. node A acquires VIPs, node B releases them
5. gratuitous ARPs announce VIPs are back on node A

### shutdown
VIPs are released (removed from interfaces) on graceful shutdown via `defer vipGroup.ReleaseAll()`. this ensures clean handoff when restarting a node

## monitoring VIP status

### web UI
the HA Status page shows a "Floating Virtual IPs" card with each VIP's held/released state, interface, and any errors

### API
```bash
# check VIP status
curl http://localhost:8067/api/v2/vips/status \
  -H "Authorization: Bearer mytoken"
```

response:
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

- `held` — the VIP manager believes it owns the IP
- `on_local` — live check of whether the IP is actually on the interface
- `error` — present if something went wrong (e.g. missing capabilities)

VIP status is also included in the `GET /api/v2/ha/status` response under the `vip` key

## capabilities

the VIP manager uses `ip addr add/del` and `arping` commands. the server needs:

| Capability | Why |
|------------|-----|
| `CAP_NET_ADMIN` | Adding and removing IP addresses from interfaces |
| `CAP_NET_RAW` | Sending gratuitous ARP packets via `arping` |

if you're already running with `CAP_NET_RAW` for conflict detection, you just need to add `CAP_NET_ADMIN`:

```bash
sudo setcap 'cap_net_raw,cap_net_bind_service,cap_net_admin+ep' /usr/local/bin/athena-dhcpd
```

or in the systemd service file:
```ini
AmbientCapabilities=CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_NET_ADMIN
```

if `ip addr add` fails (missing capability), the error is logged and reported in the VIP status API but DHCP continues to function normally. you'll see the error on the HA Status page

## firewall considerations

the VIP needs the same firewall rules as the physical IPs:

```bash
# allow DNS on the VIP
sudo ufw allow in on eth0 to 10.0.0.3 port 53 proto udp
sudo ufw allow in on eth0 to 10.0.0.3 port 53 proto tcp
```

no VRRP rules needed — there's no multicast protocol, just plain IP address management

## testing failover

1. verify VIP is on node A:
   ```bash
   ip addr show eth0 | grep 10.0.0.3
   ```

2. check VIP status via API:
   ```bash
   curl -s http://localhost:8067/api/v2/vips/status | jq .
   ```

3. query DNS through the VIP:
   ```bash
   dig @10.0.0.3 google.com
   ```

4. stop athena-dhcpd on node A:
   ```bash
   sudo systemctl stop athena-dhcpd
   ```

5. wait for failover_timeout (~10 seconds), then check VIP moved to node B:
   ```bash
   # on node B
   ip addr show eth0 | grep 10.0.0.3
   curl -s http://node-b:8067/api/v2/vips/status | jq .
   ```

6. verify DNS still works through the VIP:
   ```bash
   dig @10.0.0.3 google.com
   ```

7. restart node A and verify VIP moves back:
   ```bash
   sudo systemctl start athena-dhcpd
   # wait for recovery + VIP re-acquisition
   ip addr show eth0 | grep 10.0.0.3
   ```

## troubleshooting

**VIP not appearing on either node**
- check HA is enabled and both nodes are connected: `curl localhost:8067/api/v2/ha/status`
- check VIPs are configured: `curl localhost:8067/api/v2/vips`
- check VIP status for errors: `curl localhost:8067/api/v2/vips/status`
- check the server has CAP_NET_ADMIN: `getcap /usr/local/bin/athena-dhcpd`

**VIP on wrong node**
- check which node is ACTIVE: `curl localhost:8067/api/v2/ha/status | jq .state`
- trigger manual failover if needed: `curl -X POST localhost:8067/api/v2/ha/failover`

**DNS queries to VIP timing out**
- verify the DNS proxy is enabled and listening on 0.0.0.0: `ss -ulnp | grep :53`
- verify the VIP is on the correct interface: `ip addr show eth0`
- check that the VIP's CIDR and interface match the actual network

**VIP status shows error**
- check the logs for the specific error message
- most common: missing CAP_NET_ADMIN capability
- verify `ip` and `arping` commands are available in PATH
