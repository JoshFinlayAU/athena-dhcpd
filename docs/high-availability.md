# High Availability

active-standby failover with lease synchronisation so your network doesn't go dark when a server decides to take a nap

## overview

two athena-dhcpd nodes talk to each other over TCP. one is active (serving DHCP), the other is standby (ready to take over). leases and conflict table entries are synced in real-time so if the active node dies, the standby has everything it needs

this is NOT load balancing — only the active node serves DHCP at any given time. the standby just sits there maintaining a copy of the lease database, waiting for its moment

## setup

you need two instances of athena-dhcpd, each with its own config. the configs are identical except for the `[ha]` section

### node A (primary)
```toml
[ha]
enabled = true
role = "primary"
peer_address = "10.0.0.2:8067"    # node B's address
listen_address = "0.0.0.0:8067"
heartbeat_interval = "1s"
failover_timeout = "10s"
sync_batch_size = 100
```

### node B (secondary)
```toml
[ha]
enabled = true
role = "secondary"
peer_address = "10.0.0.1:8067"    # node A's address
listen_address = "0.0.0.0:8067"
heartbeat_interval = "1s"
failover_timeout = "10s"
sync_batch_size = 100
```

the TOML config on each node only needs `[server]` and `[api]` — everything else (subnets, pools, options, DNS, hooks, etc.) is stored in the database and automatically synced between peers. see [config sync](#config-sync) below

## state machine

the failover state machine has 5 explicit states:

| State | Description |
|-------|-------------|
| `PARTNER_UP` | Both nodes connected and communicating normally |
| `PARTNER_DOWN` | Peer heartbeat timed out, partner assumed dead |
| `ACTIVE` | This node is serving DHCP requests |
| `STANDBY` | This node is idle, maintaining lease copy |
| `RECOVERY` | Peer reconnected after being down, bulk sync in progress |

### startup behavior

- **Primary** starts in `ACTIVE` state — immediately begins serving
- **Secondary** starts in `STANDBY` state — waits for the primary

once both nodes establish a TCP connection and exchange heartbeats, both transition to `PARTNER_UP`

### failover sequence

1. Active node stops sending heartbeats (crashed, network issue, whatever)
2. Standby node's heartbeat timer expires (`failover_timeout`, default 10s)
3. Standby transitions: `PARTNER_UP` → `PARTNER_DOWN`
4. If standby is primary role: automatically transitions to `ACTIVE` and starts serving
5. If standby is secondary role: transitions to `PARTNER_DOWN` and waits (you may need to trigger manual failover)

### recovery sequence

1. Dead node comes back online
2. Peer connection re-established
3. Both nodes enter `RECOVERY` state
4. Bulk sync happens — the node that was active sends all leases and conflicts to the recovering node
5. Once bulk sync completes:
   - Primary → `ACTIVE`
   - Secondary → `STANDBY`

## config sync

configuration is replicated between peers automatically. you change something on one node (via the web UI or API) and the other node picks it up within seconds

### how it works

- every config change (subnets, defaults, conflict detection, DNS, hooks, DDNS, HA) fires an `onLocalChange` event
- the local node sends a `ConfigSync` message (type `0x0B`) to the peer containing the section name and full JSON payload
- the peer applies it to its own BoltDB via `ApplyPeerConfig` and triggers a live reload
- the peer does NOT echo the change back — no infinite loops

### adjacency push

every time a TCP connection forms between peers (initial startup, reconnect after failure, network blip recovery), whoever is currently **active** pushes their entire config to the peer. this happens:

- on normal startup (primary pushes to secondary)
- after failover recovery (the node that was active during the outage pushes to the returning node)
- on any reconnection regardless of cause

this means config is always consistent after adjacency. if you made changes on the active node while the peer was offline, those changes get pushed as soon as the peer reconnects

### conflict resolution

last-write-wins using millisecond timestamps. in practice this is fine because only one node should be active at a time, and all writes go through the active node's web UI

### what gets synced

| Section | Description |
|---------|-------------|
| `subnets` | All subnet configs including pools, reservations, options, interface bindings |
| `defaults` | Global default lease time, DNS servers, domain name |
| `conflict_detection` | Probe strategy, timeouts, ARP/ICMP settings |
| `ha` | HA settings (role, peer address, timeouts) |
| `hooks` | Script and webhook hook configurations |
| `ddns` | Dynamic DNS zones, TSIG keys, API keys |
| `dns` | DNS proxy settings, forwarders, filter lists, static records |

bootstrap config (`[server]` and `[api]`) is NOT synced — each node keeps its own

## lease synchronisation

leases are synced event-driven (not polling). whenever a lease changes on the active node:

1. DHCPACK/release/expire happens
2. Lease update pushed to peer over TCP
3. Peer updates its local BoltDB

during normal operation this is near-instant. the peer's lease database is always close to the active node's

### bulk sync

when a node reconnects after being offline, it does a bulk sync:

1. Active node sends `BULK_START` message with total counts
2. Sends all leases in batches of `sync_batch_size` (default 100)
3. Sends all conflict table entries
4. Sends `BULK_END` message
5. FSM transitions out of RECOVERY

## conflict table sync

the conflict table is synced alongside leases using the same mechanism. conflict detections, DHCPDECLINE events, and permanent flags all propagate to the peer. so if the active node detects a conflict on 192.168.1.50, the standby knows about it too

## wire protocol

messages between peers use a simple length-prefixed JSON format over TCP:

```
[4 bytes: message length (big-endian uint32)][JSON payload]
```

message types:
- `0x01` — Heartbeat (state, lease count, sequence number, uptime)
- `0x02` — Lease Update
- `0x03` — Bulk Start
- `0x04` — Bulk Data
- `0x05` — Bulk End
- `0x06` — Failover Claim
- `0x07` — Failover Ack
- `0x08` — State Request
- `0x09` — Conflict Update
- `0x0A` — Conflict Bulk
- `0x0B` — Config Sync (section name + JSON payload + timestamp)

max message size is 1MB (more than enough, lease updates are tiny)

## heartbeats

sent every `heartbeat_interval` (default 1s). each heartbeat includes:

- current HA state
- lease count
- sequence number (for conflict resolution)
- uptime

if no heartbeat is received within `failover_timeout` (default 10s), the peer is declared down

## manual failover

you can force a failover via the API:

```bash
curl -X POST http://localhost:8067/api/v2/ha/failover \
  -H "Authorization: Bearer mytoken"
```

this forces the current node to `ACTIVE` state and sends a failover claim to the peer, which transitions to `STANDBY`

the web UI also has a big shiny failover button on the HA status page. try not to press it by accident

## TLS

if your HA peers communicate over an untrusted network (or you're just paranoid, which is healthy), enable TLS:

```toml
[ha.tls]
enabled = true
cert_file = "/etc/athena-dhcpd/tls/server.crt"
key_file = "/etc/athena-dhcpd/tls/server.key"
ca_file = "/etc/athena-dhcpd/tls/ca.crt"
```

both nodes need valid certificates. the CA file is used to verify the peer's certificate

## events

HA state changes fire events through the event bus:

| Event | When |
|-------|------|
| `ha.failover` | Any state transition (includes old and new state) |
| `ha.sync_complete` | Bulk sync finished |

these are available to hooks. good for alerting — you probably want to know when a failover happens

## metrics

- `athena_dhcpd_ha_state{state,role}` — current state (gauge, 1 = current)
- `athena_dhcpd_ha_heartbeats_sent_total` — heartbeats sent
- `athena_dhcpd_ha_heartbeats_received_total` — heartbeats received
- `athena_dhcpd_ha_sync_operations_total{type}` — sync ops (lease_update, conflict_update, config_sync)
- `athena_dhcpd_ha_sync_errors_total` — sync failures

## floating IP for DNS

if you're running the DNS proxy in HA, clients need a stable IP to send DNS queries to. see [HA with Floating IP for DNS Proxy](ha-floating-ip.md) for a full guide on using keepalived (or event hooks) to move a virtual IP between nodes on failover

## things to know

- config is synced automatically between peers — you only need to manage one node's web UI
- the lease database path should be different on each node (they each maintain their own BoltDB)
- network partition = split brain risk. the `failover_timeout` is your safety margin. make it long enough that transient network blips don't cause unnecessary failovers
- there's no quorum mechanism (its 2 nodes). if both nodes think they're active, clients might get duplicate offers. this is temporary and resolves once the partition heals
- connection uses exponential backoff for reconnection (1s → 2s → 4s → ... → 30s max)
