# Project: `athena-dhcpd` — RFC-Compliant HA DHCP Server in Go

## Overview

A production-grade, RFC-compliant DHCPv4 server written in Go with built-in high availability via peer lease synchronisation. Self-contained with no external database dependencies — leases persist to local disk using BoltDB (embedded key-value store). Configuration uses TOML for clarity and simplicity.

Includes an embedded web management interface served directly from the binary, dynamic DNS integration with all major DNS servers, a lease event hook system for triggering external scripts and webhooks on DHCP events, and pre-assignment IP conflict detection via ARP probing.

This server targets deployment on carrier and MSP networks where reliability, simplicity, and standards compliance matter more than feature bloat.

---

## Architecture Summary

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           athena-dhcpd                                   │
│                                                                          │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────────┐  ┌────────────┐   │
│  │  Config  │  │  DHCP Engine │  │   HA Sync Engine  │  │  Web UI    │   │
│  │  (TOML)  │──│  (RFC 2131)  │──│  (Peer-to-Peer)   │  │ (Embedded) │   │
│  └──────────┘  └──────┬───────┘  └─────────┬─────────┘  └─────┬──────┘   │
│                       │                    │                  │          │
│              ┌────────▼────────┐  ┌────────▼──────────┐  ┌─────▼──────┐  │
│              │  Lease Manager  │──│  Sync Protocol    │  │  HTTP API  │  │
│              │  (BoltDB)       │  │  (TCP + TLS)      │  │  (JSON)    │  │
│              └──┬──────┬───────┘  └───────────────────┘  └────────────┘  │
│                 │      │                                                 │
│     ┌───────────▼─┐   ┌▼─────────────────────────────────┐               │
│     │  Conflict   │   │        Event Bus (Fan-out)       │               │
│     │  Detector   │   └───┬──────────┬──────────┬────────┘               │
│     │ (ARP/ICMP)  │       │          │          │                        │
│     └─────────────┘ ┌─────▼────┐ ┌───▼──────┐ ┌─▼───────────┐            │
│                     │  Script  │ │ Webhook  │ │ Dynamic DNS │            │
│                     │  Hooks   │ │ Hooks    │ │ Updater     │            │
│                     └──────────┘ └──────────┘ └─────────────┘            │
│                                                                          │
│  ┌───────────────┐  ┌─────────────┐  ┌───────────────┐                   │
│  │ Option Engine │  │ Relay Agent │  │  Metrics      │                   │
│  │ (All RFCs)    │  │ (Opt 82)    │  │  (Prometheus) │                   │
│  └───────────────┘  └─────────────┘  └───────────────┘                   │
└──────────────────────────────────────────────────────────────────────────┘
```

### HA Model

Active-passive with automatic failover. Both peers maintain synchronised lease databases over a dedicated TCP (TLS-optional) channel. The active node processes DHCP requests; the passive node receives lease state updates in real-time. On failure detection (heartbeat timeout), the passive node promotes itself to active.

```
┌──────────────┐       Lease Sync (TCP/TLS)       ┌──────────────┐
│   Node A     │◄────────────────────────────────►│   Node B     │
│   (Active)   │       Heartbeat (UDP)            │  (Standby)   │
│              │◄────────────────────────────────►│              │
│  BoltDB      │                                  │  BoltDB      │
│  (primary)   │                                  │  (replica)   │
└──────────────┘                                  └──────────────┘
```

---

## RFC Compliance Matrix

The server MUST implement the following RFCs. Each RFC should be referenced by number in code comments where the relevant logic is implemented.

### Core Protocol

| RFC  | Title                                    | Priority | Notes                                                        |
| ---- | ---------------------------------------- | -------- | ------------------------------------------------------------ |
| 2131 | Dynamic Host Configuration Protocol      | **MUST** | Full DORA cycle, INIT-REBOOT, renew, rebind. §4.4.1: server SHOULD probe before OFFER |
| 2132 | DHCP Options and BOOTP Vendor Extensions | **MUST** | All standard options (see Option Engine below)               |
| 951  | Bootstrap Protocol (BOOTP)               | **MUST** | BOOTP compatibility layer                                    |

### Options & Extensions

| RFC  | Title                                           | Priority   | Notes                                     |
| ---- | ----------------------------------------------- | ---------- | ----------------------------------------- |
| 3046 | DHCP Relay Agent Information Option (Opt 82)    | **MUST**   | Sub-options 1 (Circuit ID), 2 (Remote ID) |
| 3442 | Classless Static Route Option (Opt 121)         | **MUST**   | Replaces option 33 for CIDR routes        |
| 3004 | User Class Option (Opt 77)                      | **SHOULD** | Pool selection by user class              |
| 3011 | IPv4 Subnet Selection Option (Opt 118)          | **SHOULD** | Relay scenarios with multiple subnets     |
| 3527 | Link Selection Sub-Option (Opt 82 sub 5)        | **SHOULD** | Override giaddr for relay                 |
| 4361 | Node-specific Client Identifiers                | **SHOULD** | DUID-based client identification          |
| 6842 | Client Identifier Option in Server Replies      | **MUST**   | Echo client-id back in responses          |
| 4702 | DHCP Client FQDN Option (Opt 81)                | **MUST**   | Client FQDN for DDNS integration          |
| 3925 | Vendor-Identifying Vendor Options (Opt 124/125) | **SHOULD** | Multi-vendor environments                 |
| 2241 | DHCP Options for Novell Directory Services      | MAY        | Legacy, implement if time permits         |
| 2242 | NetWare/IP Domain Name and Information          | MAY        | Legacy                                    |

### Dynamic DNS

| RFC  | Title                                        | Priority   | Notes                             |
| ---- | -------------------------------------------- | ---------- | --------------------------------- |
| 2136 | Dynamic Updates in the DNS (DNS UPDATE)      | **MUST**   | Core DDNS protocol                |
| 2845 | Secret Key Transaction Authentication (TSIG) | **MUST**   | Secure DNS updates                |
| 4701 | Resolution of FQDN Conflicts                 | **SHOULD** | DHCID RR for conflict detection   |
| 4703 | Resolution of DNS Name Conflicts             | **SHOULD** | Name conflict handling procedures |

### Conflict Detection

| RFC         | Title                             | Priority   | Notes                                             |
| ----------- | --------------------------------- | ---------- | ------------------------------------------------- |
| 5765        | Security Vulnerability in DHCP    | **MUST**   | Starvation attack mitigation                      |
| 2131 §4.4.1 | Server probe before OFFER         | **MUST**   | ARP/ICMP probe before offering an IP              |
| 826         | ARP (Address Resolution Protocol) | **MUST**   | Raw ARP probe for local subnet conflict detection |
| 792         | ICMP Echo (Ping)                  | **SHOULD** | Fallback probe for relayed/remote subnets         |

### Security & Operations

| RFC  | Title                            | Priority   | Notes                           |
| ---- | -------------------------------- | ---------- | ------------------------------- |
| 3118 | Authentication for DHCP Messages | **SHOULD** | Delayed authentication protocol |
| 7724 | Active DHCPv4 Lease Query        | MAY        | Bulk lease query for monitoring |
| 6926 | DHCPv4 Bulk Leasequery           | MAY        | Large-scale lease enumeration   |

### Behavioural

| RFC  | Title                              | Priority   | Notes          |
| ---- | ---------------------------------- | ---------- | -------------- |
| 4436 | Detecting Network Attachment (DNA) | **SHOULD** | Fast reconnect |

---

## IP Conflict Detection

### Why This Matters

Duplicate IP assignment is one of the most disruptive failures in a network. The server MUST verify that an IP address is not already in use before offering it to a client. This catches:

- Stale leases from a previous server or manual static assignments
- Devices with statically configured IPs that overlap pool ranges
- Split-brain HA scenarios where both nodes assigned the same IP
- Network devices not participating in DHCP that hold addresses

### Detection Methods

The conflict detector supports multiple probe methods, selected automatically based on network topology:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Conflict Detector                             │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Is candidate IP on a directly-attached subnet?           │   │
│  └──────────┬───────────────────────────┬───────────────────┘   │
│             YES                         NO (relayed)            │
│              │                           │                      │
│     ┌────────▼────────┐       ┌──────────▼──────────┐          │
│     │   ARP Probe     │       │    ICMP Echo Probe   │          │
│     │                 │       │    (ping)             │          │
│     │  Raw socket     │       │                      │          │
│     │  Broadcast ARP  │       │  Sent via normal     │          │
│     │  who-has on     │       │  routing toward      │          │
│     │  DHCP interface │       │  candidate IP        │          │
│     └────────┬────────┘       └──────────┬──────────┘          │
│              │                            │                     │
│     ┌────────▼────────────────────────────▼──────────┐         │
│     │              Result                             │         │
│     │                                                 │         │
│     │  Reply received → CONFLICT DETECTED             │         │
│     │    → Mark IP as conflicted, skip it, try next   │         │
│     │    → Log warning with responder's MAC           │         │
│     │    → Fire conflict event                        │         │
│     │                                                 │         │
│     │  Timeout (no reply) → IP is CLEAR               │         │
│     │    → Proceed with DHCPOFFER                     │         │
│     └─────────────────────────────────────────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

### ARP Probe (Local Subnets) — Primary Method

Used when the candidate IP is on a subnet directly attached to the server's DHCP interface. This is the most reliable method.

**How it works:**

1. Server opens a raw socket (AF_PACKET) bound to the DHCP interface
2. Sends an ARP request: "Who has {candidate IP}? Tell {server IP}"
3. Waits for an ARP reply for the configured timeout
4. If a reply arrives: the IP is in use — conflict detected
5. If timeout expires with no reply: IP is clear

**Packet format (standard ARP Request per RFC 826):**

```
Hardware Type:  0x0001 (Ethernet)
Protocol Type:  0x0800 (IPv4)
HW Addr Len:    6
Proto Addr Len: 4
Operation:      1 (ARP Request)
Sender MAC:     <server interface MAC>
Sender IP:      <server IP on DHCP interface>
Target MAC:     00:00:00:00:00:00
Target IP:      <candidate IP to probe>
```

Ethernet frame is sent to broadcast (ff:ff:ff:ff:ff:ff).

**Implementation notes:**

- Use a raw AF_PACKET socket for sending and receiving ARP
- The server maintains a single persistent raw socket per DHCP interface (not opened/closed per probe)
- ARP replies are filtered by target IP to avoid false matches
- Requires `CAP_NET_RAW` capability (or root) — document in deployment guide

### ICMP Echo Probe (Relayed/Remote Subnets) — Fallback Method

Used when the candidate IP is on a remote subnet (client reached via relay agent / giaddr). ARP doesn't work across L3 boundaries, so we fall back to ICMP echo (ping).

**How it works:**

1. Server sends an ICMP Echo Request to the candidate IP
2. Waits for an ICMP Echo Reply for the configured timeout
3. If a reply arrives: conflict detected
4. If timeout expires: IP is considered clear

**Limitations (documented and logged):**

- Some hosts block ICMP — false negatives are possible
- Firewalls between server and subnet may drop ICMP
- ICMP is less reliable than ARP for conflict detection
- For relayed subnets, ARP-via-relay (unicast ARP through the relay) is a future enhancement

**Implementation notes:**

- Use Go's `net.ListenPacket("ip4:icmp", "0.0.0.0")` for raw ICMP
- Requires `CAP_NET_RAW` capability
- ICMP ID field set to PID, sequence incremented per probe for correlation

### Gratuitous ARP / ARP Announcement (Post-Assignment)

After successfully assigning a lease (sending DHCPACK), the server optionally sends a Gratuitous ARP to update ARP caches on the local network. This helps when the IP was previously used by a different device.

**Packet format (Gratuitous ARP):**

```
Operation:      1 (ARP Request)
Sender MAC:     <client MAC from lease>
Sender IP:      <assigned IP>
Target MAC:     ff:ff:ff:ff:ff:ff
Target IP:      <assigned IP>      (same as sender — this is what makes it gratuitous)
```

This is optional and configurable (`send_gratuitous_arp = true`). Only applicable for local subnets.

### DHCPDECLINE Handling (Client-Side Conflict Detection)

Per RFC 2131, clients perform their own conflict detection (usually ARP) after receiving DHCPACK. If the client detects a conflict, it sends DHCPDECLINE. The server MUST:

1. Mark the declined IP as **conflicted** in the conflict table
2. Remove the lease for that IP
3. Log the conflict with full context (IP, client MAC, subnet)
4. Fire a `conflict.decline` event through the event bus
5. The conflicted IP remains blocked for `conflict_hold_time` (configurable, default 1 hour)
6. After hold time expires, the IP is probed again before reuse

### Conflict Table

The server maintains a conflict table (in BoltDB + in-memory cache) that tracks IPs known to have conflicts:

```
Conflict Record:
{
  "ip": "10.10.0.105",
  "detected_at": "2025-02-21T14:30:00Z",
  "detection_method": "arp_probe",        // arp_probe, icmp_probe, client_decline
  "responder_mac": "aa:bb:cc:11:22:33",   // MAC that responded (if ARP), empty for ICMP
  "hold_until": "2025-02-21T15:30:00Z",   // When to retry this IP
  "subnet": "10.10.0.0/24",
  "probe_count": 1,                        // How many times this IP has conflicted
  "resolved": false
}
```

**Conflict lifecycle:**

1. IP selected for offer → probe (ARP or ICMP)
2. Probe response received → add to conflict table, select next IP, repeat
3. Hold timer expires → IP eligible for allocation again, but will be re-probed
4. If an IP conflicts repeatedly (configurable threshold, default 3), it is permanently
   flagged and requires manual resolution via API/UI
5. Manual clear via API: `DELETE /api/v1/conflicts/:ip`

### Probe Concurrency & Performance

Conflict detection adds latency to the DORA cycle. The design minimises this impact:

**Sequential probe (default):**

- One probe at a time, per DHCPDISCOVER
- Adds `probe_timeout` (default 500ms) to the DISCOVER→OFFER path per candidate IP
- If first candidate is clear, total added latency = one probe timeout
- If first candidate conflicts, try next IP with another probe
- Max probes per discover = `max_probes_per_discover` (default 3), then fall back to offering without probe (with warning log)

**Optimistic parallel probe (optional, configurable):**

- When a DHCPDISCOVER arrives, pre-probe the top N candidate IPs in parallel
- First clear IP is used for the OFFER
- Adds only one probe timeout regardless of conflicts
- Uses more raw socket bandwidth but dramatically improves worst-case latency
- Enabled via `probe_strategy = "parallel"` with `parallel_probe_count = 3`

**Probe cache:**

- Recently probed IPs that were clear are cached for `probe_cache_ttl` (default 10s)
- If the same IP is selected within the cache TTL, skip the probe
- Cache is invalidated on any DHCPDECLINE for that IP
- Useful for high-churn environments where multiple DISCOVERs arrive in quick succession

### Probe Timing Considerations

```
                        Without Conflict Detection
DISCOVER ──────────────────────────────────────────── OFFER
                        ~1ms (lookup + allocate)

                        With ARP Probe (no conflict)
DISCOVER ──── ARP probe ─── timeout (500ms) ──────── OFFER
                        ~500ms worst case

                        With ARP Probe (conflict, retry once)
DISCOVER ──── ARP probe ─── reply! ─── next IP ─── ARP probe ─── timeout ──── OFFER
                        ~500ms + ~500ms = ~1s worst case

                        With Parallel Probe (3 candidates)
DISCOVER ──── ARP probe x3 parallel ─── first clear ──── OFFER
                        ~500ms worst case regardless of conflicts
```

### Events

The conflict detector fires events through the event bus:

| Event                | Trigger                            | Notes                              |
| -------------------- | ---------------------------------- | ---------------------------------- |
| `conflict.detected`  | ARP/ICMP probe got a response      | Includes IP, method, responder MAC |
| `conflict.decline`   | Client sent DHCPDECLINE            | Includes IP, client MAC            |
| `conflict.resolved`  | Hold timer expired or manual clear | Includes IP, resolution method     |
| `conflict.permanent` | IP exceeded max conflict count     | Requires manual intervention       |

These events are available to script hooks, webhook hooks, and the web UI event stream.

---

## DHCP Options — Full Implementation

The option engine MUST support encoding/decoding ALL standard DHCP options (1–254). Options are defined as typed entries with serialisation logic per type. The engine uses a registry pattern.

### Option Type System

```go
type OptionType int

const (
    TypeIP        OptionType = iota  // Single IPv4 address (4 bytes)
    TypeIPList                        // Multiple IPv4 addresses (N*4 bytes)
    TypeUint8
    TypeUint16
    TypeUint32
    TypeInt32
    TypeBool                          // 1 byte, 0x00 or 0x01
    TypeString                        // Variable-length ASCII
    TypeBytes                         // Raw bytes
    TypeIPMask                        // IP + subnet mask pairs
    TypeCIDRRoutes                    // RFC 3442 encoded routes
)
```

### Required Options (non-exhaustive, ALL 1-254 must be handleable)

Key options that MUST have first-class config support:

| Code | Name                    | Type       | Config Key            |
| ---- | ----------------------- | ---------- | --------------------- |
| 1    | Subnet Mask             | IP         | `subnet_mask`         |
| 3    | Router                  | IPList     | `routers`             |
| 6    | DNS Servers             | IPList     | `dns_servers`         |
| 12   | Hostname                | String     | `hostname`            |
| 15   | Domain Name             | String     | `domain_name`         |
| 28   | Broadcast Address       | IP         | `broadcast`           |
| 42   | NTP Servers             | IPList     | `ntp_servers`         |
| 43   | Vendor Specific         | Bytes      | `vendor_specific`     |
| 51   | Lease Time              | Uint32     | `lease_time`          |
| 54   | Server Identifier       | IP         | (auto from interface) |
| 58   | Renewal Time (T1)       | Uint32     | `renewal_time`        |
| 59   | Rebind Time (T2)        | Uint32     | `rebind_time`         |
| 60   | Vendor Class ID         | String     | `vendor_class`        |
| 66   | TFTP Server             | String     | `tftp_server`         |
| 67   | Bootfile Name           | String     | `bootfile`            |
| 81   | Client FQDN             | Special    | (parsed for DDNS)     |
| 82   | Relay Agent Info        | Bytes      | (parsed from relay)   |
| 121  | Classless Static Routes | CIDRRoutes | `static_routes`       |
| 150  | TFTP Server Address     | IPList     | `tftp_addresses`      |

Any option not explicitly configured MUST still be decodable from packets and passable through the option engine.

---

## Lease Event Hook System

### Event Types

The server fires events at every stage of the DHCP lifecycle. Each event carries the full lease context.

| Event                | Trigger                               | Notes                                              |
| -------------------- | ------------------------------------- | -------------------------------------------------- |
| `lease.discover`     | DHCPDISCOVER received                 | Before offer, includes requested IP if any         |
| `lease.offer`        | DHCPOFFER sent                        | Includes offered IP and options                    |
| `lease.ack`          | DHCPACK sent (new lease)              | First-time assignment — lease is now active        |
| `lease.renew`        | DHCPACK sent (renewal)                | Existing lease renewed with same or updated params |
| `lease.nak`          | DHCPNAK sent                          | Request rejected — includes reason                 |
| `lease.release`      | DHCPRELEASE received                  | Client voluntarily released the lease              |
| `lease.decline`      | DHCPDECLINE received                  | Client detected IP conflict                        |
| `lease.expire`       | Lease TTL reached                     | GC detected expiry, lease removed                  |
| `conflict.detected`  | Server-side probe got response        | ARP/ICMP conflict, includes responder MAC          |
| `conflict.decline`   | Client DHCPDECLINE received           | Client-side conflict detection                     |
| `conflict.resolved`  | Conflict hold expired or manual clear | IP available again                                 |
| `conflict.permanent` | IP exceeded max conflict threshold    | Requires manual intervention                       |
| `ha.failover`        | Node role changed                     | Includes old/new role, peer state                  |
| `ha.sync_complete`   | Bulk sync finished                    | Full lease sync from peer completed                |

### Hook Execution Model

```
┌──────────────┐     ┌────────────────────┐     ┌──────────────────┐
│ Lease Manager │────►│     Event Bus      │────►│  Hook Dispatcher │
│  (produces    │     │  (buffered chan,    │     │  (fan-out to     │
│   events)     │     │   non-blocking)    │     │   all hooks)     │
└──────────────┘     └────────────────────┘     └───────┬──────────┘
                                                        │
                              ┌──────────────────────────┼──────────────┐
                              │                          │              │
                       ┌──────▼──────┐  ┌────────────────▼┐  ┌─────────▼──────┐
                       │   Script    │  │    Webhook       │  │  DDNS Updater  │
                       │   Runner    │  │    Poster        │  │  (internal)    │
                       └─────────────┘  └─────────────────┘  └────────────────┘
```

**Critical design rules:**

- The event bus is a buffered Go channel — events are NEVER blocking on the DHCP hot path
- If the buffer is full, events are dropped with a warning log and a metric increment
- Script hooks run asynchronously in a goroutine pool (configurable concurrency)
- Script execution timeout is enforced (default 10s, configurable)
- Webhook hooks use an HTTP client pool with timeout and retry
- DDNS is a built-in subscriber to the event bus (not a script)
- Hook failures NEVER affect DHCP operation — log error, increment metric, move on

### Script Hook Interface

Scripts receive lease data as **environment variables** and the full event as **JSON on stdin**.

Environment variables passed to every hook script:

```
ATHENA_EVENT=lease.ack
ATHENA_IP=10.10.0.105
ATHENA_MAC=aa:bb:cc:dd:ee:01
ATHENA_HOSTNAME=device-1
ATHENA_CLIENT_ID=01aabbccddeef1
ATHENA_SUBNET=10.10.0.0/24
ATHENA_LEASE_START=1708500000
ATHENA_LEASE_EXPIRY=1708528800
ATHENA_LEASE_DURATION=28800
ATHENA_GATEWAY=10.10.0.1
ATHENA_DNS_SERVERS=1.1.1.1,8.8.8.8
ATHENA_DOMAIN=office.athena.net.au
ATHENA_RELAY_AGENT_CIRCUIT_ID=eth0/1/3
ATHENA_RELAY_AGENT_REMOTE_ID=switch-floor2
ATHENA_SERVER_ID=10.10.0.1
ATHENA_OLD_IP=10.10.0.99          # only on renewal if IP changed
ATHENA_FQDN=device-1.office.athena.net.au
ATHENA_VENDOR_CLASS=Cisco
ATHENA_CONFLICT_METHOD=arp_probe   # only on conflict.* events
ATHENA_CONFLICT_RESPONDER_MAC=aa:bb:cc:11:22:33  # only on conflict.detected
```

JSON on stdin (full event payload):

```json
{
  "event": "lease.ack",
  "timestamp": "2025-02-21T14:30:00Z",
  "lease": {
    "ip": "10.10.0.105",
    "mac": "aa:bb:cc:dd:ee:01",
    "client_id": "01aabbccddeef1",
    "hostname": "device-1",
    "fqdn": "device-1.office.athena.net.au",
    "subnet": "10.10.0.0/24",
    "pool": "10.10.0.100-10.10.0.200",
    "start": 1708500000,
    "expiry": 1708528800,
    "state": "active",
    "options": {
      "routers": ["10.10.0.1"],
      "dns_servers": ["1.1.1.1", "8.8.8.8"],
      "domain_name": "office.athena.net.au"
    },
    "relay": {
      "giaddr": "10.10.0.1",
      "circuit_id": "eth0/1/3",
      "remote_id": "switch-floor2"
    }
  },
  "server": {
    "node_id": "node-a",
    "ha_role": "active"
  }
}
```

Conflict events include additional fields:

```json
{
  "event": "conflict.detected",
  "timestamp": "2025-02-21T14:30:00Z",
  "conflict": {
    "ip": "10.10.0.105",
    "subnet": "10.10.0.0/24",
    "detection_method": "arp_probe",
    "responder_mac": "aa:bb:cc:11:22:33",
    "probe_count": 1,
    "hold_until": "2025-02-21T15:30:00Z",
    "intended_client_mac": "dd:ee:ff:00:11:22"
  }
}
```

Script exit codes:

- `0` — Success (logged at debug level)
- Non-zero — Failure (logged at warn level with stderr captured)
- Scripts killed after timeout — logged at error level

### Webhook Hook Interface

Webhooks POST the same JSON payload as stdin for scripts, with additional headers:

```
POST /dhcp/events HTTP/1.1
Content-Type: application/json
X-Athena-Event: lease.ack
X-Athena-Signature: sha256=<HMAC of body with shared secret>
X-Athena-Node: node-a
```

Webhook retries: 3 attempts with exponential backoff (1s, 5s, 15s). Configurable.

---

## Dynamic DNS Integration

### Architecture

DDNS is a first-class, built-in feature — NOT a script hook. It subscribes directly to the event bus and performs DNS updates inline (but still asynchronously to the DHCP path).

```
┌──────────────┐     ┌────────────────┐     ┌───────────────────────────┐
│  Event Bus   │────►│  DDNS Manager  │────►│  DNS Server               │
│              │     │                │     │                           │
│  lease.ack   │     │  - Build FQDN  │     │  BIND 9 (RFC 2136/TSIG)  │
│  lease.renew │     │  - Create A/PTR│     │  Knot DNS (RFC 2136/TSIG) │
│  lease.release│    │  - Delete on   │     │  PowerDNS (RFC 2136 or    │
│  lease.expire│     │    release/exp │     │     native HTTP API)      │
│              │     │  - DHCID check │     │  Windows DNS (RFC 2136)   │
│              │     │  - Conflict    │     │  Technitium (HTTP API)    │
│              │     │    resolution  │     │  CoreDNS (RFC 2136)       │
└──────────────┘     └────────────────┘     └───────────────────────────┘
```

### Supported DNS Servers & Methods

| DNS Server             | Update Method               | Auth                                      | Notes                                                     |
| ---------------------- | --------------------------- | ----------------------------------------- | --------------------------------------------------------- |
| BIND 9                 | RFC 2136 (DNS UPDATE)       | TSIG (HMAC-MD5, HMAC-SHA256, HMAC-SHA512) | Primary target, most common                               |
| Knot DNS               | RFC 2136 (DNS UPDATE)       | TSIG                                      | Same wire protocol as BIND                                |
| PowerDNS Authoritative | RFC 2136 OR native HTTP API | TSIG or API key                           | HTTP API preferred for PowerDNS — more reliable           |
| Windows DNS            | RFC 2136 (DNS UPDATE)       | TSIG or GSS-TSIG                          | GSS-TSIG for AD environments (stretch goal)               |
| Technitium DNS         | HTTP REST API               | API token                                 | Popular self-hosted DNS, native API integration           |
| CoreDNS                | RFC 2136 (DNS UPDATE)       | TSIG                                      | When using the `dynamic` plugin                           |
| Pi-hole / dnsmasq      | Script hook fallback        | N/A                                       | No update protocol — use script hooks to write hosts file |

### DDNS Behaviour

**On `lease.ack` (new lease):**

1. Construct FQDN: `{hostname}.{domain_name}` or use client-supplied FQDN (option 81)
2. Check for conflicts via DHCID RR (RFC 4701) if configured
3. Send DNS UPDATE: Add A record (forward zone) + PTR record (reverse zone)
4. Store FQDN in lease record for later cleanup

**On `lease.renew`:**

1. If hostname or FQDN changed, remove old records, add new ones
2. If unchanged, optionally refresh TTL (configurable)

**On `lease.release` or `lease.expire`:**

1. Remove A record from forward zone
2. Remove PTR record from reverse zone
3. Remove DHCID RR if present

**FQDN Construction Priority (configurable):**

1. Client-supplied FQDN via option 81 (if `allow_client_fqdn = true`)
2. Client-supplied hostname (option 12) + configured domain → `hostname.domain`
3. MAC-based fallback: `dhcp-aabbccddeef0.domain` (if `fallback_to_mac = true`)
4. Skip DDNS if no hostname available and fallback disabled

**Conflict Resolution (RFC 4701/4703):**

- Use DHCID resource record to prove ownership of a DNS name
- If an A record exists with a different DHCID, the update is a conflict
- Configurable policy: `overwrite`, `skip`, or `append` (multiple A records)

### RFC 2136 Update Wire Format

The DDNS module builds DNS UPDATE packets per RFC 2136:

```
Header: OPCODE=UPDATE
Zone Section:     office.athena.net.au. IN SOA
Prerequisite:     (optional DHCID check)
Update Section:   device-1.office.athena.net.au. 300 IN A 10.10.0.105
                  105.0.10.10.in-addr.arpa. 300 IN PTR device-1.office.athena.net.au.
Additional:       (TSIG signature)
```

### PowerDNS HTTP API Integration

```
PATCH /api/v1/servers/localhost/zones/office.athena.net.au.
X-API-Key: <key>
Content-Type: application/json

{
  "rrsets": [
    {
      "name": "device-1.office.athena.net.au.",
      "type": "A",
      "ttl": 300,
      "changetype": "REPLACE",
      "records": [{"content": "10.10.0.105", "disabled": false}]
    }
  ]
}
```

### Technitium DNS API Integration

```
POST /api/zones/records/add
?token=<api_token>
&domain=device-1.office.athena.net.au
&zone=office.athena.net.au
&type=A
&ipAddress=10.10.0.105
&ttl=300
&overwrite=true
```

---

## Embedded Web Interface

### Architecture

The web UI is a single-page application (SPA) built with React + TypeScript + Tailwind CSS, compiled to static assets at build time and embedded into the Go binary via `go:embed`. Zero external dependencies at runtime — no Node.js, no CDN, no separate web server.

```
┌────────────────────────────────────────────────────┐
│                  Go Binary                          │
│                                                     │
│  ┌─────────────┐     ┌──────────────────────────┐  │
│  │ go:embed     │     │      HTTP Router          │  │
│  │ web/dist/*   │────►│                          │  │
│  └─────────────┘     │  /              → SPA     │  │
│                      │  /api/v1/*      → JSON API│  │
│                      │  /metrics       → Prom    │  │
│                      └──────────────────────────┘  │
│                                                     │
└────────────────────────────────────────────────────┘
```

### Build Pipeline

```
web/                          # Frontend source (not embedded)
├── src/
│   ├── App.tsx
│   ├── pages/
│   │   ├── Dashboard.tsx
│   │   ├── Leases.tsx
│   │   ├── Reservations.tsx
│   │   ├── Subnets.tsx
│   │   ├── Config.tsx
│   │   ├── Events.tsx
│   │   ├── Conflicts.tsx
│   │   └── HAStatus.tsx
│   ├── components/
│   │   ├── Layout.tsx         # Shell with sidebar nav
│   │   ├── LeaseTable.tsx
│   │   ├── PoolBar.tsx        # Visual pool utilisation
│   │   ├── SubnetCard.tsx
│   │   ├── EventLog.tsx
│   │   ├── ConfigEditor.tsx
│   │   ├── ConflictTable.tsx
│   │   └── HAIndicator.tsx
│   └── api/
│       └── client.ts          # Typed API client
├── index.html
├── tailwind.config.js
├── vite.config.ts
├── tsconfig.json
└── package.json

# Build step (Makefile target):
#   cd web && npm ci && npm run build
#   Output: web/dist/  (index.html + JS + CSS, all hashed)

# Embedded in Go:
#   //go:embed web/dist/*
#   var webAssets embed.FS
```

The `Makefile` has a `build-web` target that builds the frontend, and the main `build` target depends on it. CI builds both. For development, `npm run dev` runs a Vite dev server that proxies `/api` to the running Go binary.

### Web UI Pages

#### 1. Dashboard

- **Pool utilisation** — visual bars per subnet/pool showing used/available/reserved/conflicted
- **Active lease count** — total and per subnet
- **DHCP packets/sec** — real-time counter (polling API every 2s)
- **HA status** — active/standby indicator, peer connectivity, last sync time
- **Recent events** — last 20 lease events (live-updating)
- **Active conflicts** — count of IPs currently in conflict, with quick-link to conflicts page
- **Alerts** — pools above 90% utilisation, HA peer down, high NAK rate, new conflicts

#### 2. Leases

- **Sortable, filterable table** of all active leases
- Columns: IP, MAC, Hostname, FQDN, Subnet, State, Lease Start, Expiry, Remaining
- **Search** by IP, MAC, hostname, subnet
- **Filters**: subnet dropdown, state (active/offered/expired), expiry window
- **Actions per lease**: Force release, convert to reservation, view full options
- **Export**: CSV download of filtered lease table

#### 3. Reservations (Static Leases)

- **CRUD interface** for managing host reservations
- Add reservation: MAC or client-id, IP, hostname, per-host option overrides
- Edit existing reservations inline
- Delete with confirmation
- **Import/Export**: Bulk import from CSV, export current reservations
- **Validation**: Checks IP is within a configured subnet, no conflicts with pools or other reservations
- Changes write to the config file and trigger a live reload

#### 4. Subnets & Pools

- **Visual overview** of all configured subnets
- Per-subnet: network, pool ranges, utilisation, option summary
- **Pool details**: range, match criteria (option 82, vendor class), utilisation
- Ability to add/edit/remove pools within subnets (writes config, triggers reload)

#### 5. Configuration

- **Read-only view** of the full running configuration (TOML, syntax highlighted)
- **Editable sections** for safe modifications:
  - Reservations (see page 3)
  - Pool ranges
  - Default options
  - DDNS settings
  - Hook configuration
  - Conflict detection settings
- **Config validation** — validates changes before applying
- **Apply button** — writes changes to disk and sends SIGHUP to reload
- **Config diff** — shows what will change before applying
- **Backup** — auto-backup previous config on every write

#### 6. Events & Hooks

- **Live event log** — WebSocket stream of lease and conflict events in real-time
- Filterable by event type (including conflict.*), subnet, MAC, IP
- **Hook status** — list of configured hooks with last execution status
- Per-hook: last run time, success/fail count, last error message
- **Test hook** — trigger a test event to verify hook configuration

#### 7. Conflicts

- **Active conflicts table** — all IPs currently marked as conflicted
- Columns: IP, Subnet, Detection Method, Responder MAC, Detected At, Hold Until, Probe Count, Status
- **Actions**: Clear conflict (return IP to pool), permanently exclude IP from pool
- **History**: recently resolved conflicts
- **Stats**: conflict rate over time, most frequent offending MACs/subnets

#### 8. HA Status

- **Peer status**: connected/disconnected, role (active/standby), last heartbeat
- **Sync status**: last sync time, pending sync count, sync lag
- **Lease comparison**: count on each node, any discrepancies flagged
- **Manual failover** button with confirmation dialog
- **Sync history**: recent sync events with timing

### API Endpoints (JSON)

The web UI is backed by the same HTTP API available for automation. All endpoints return JSON.

#### Leases

```
GET    /api/v1/leases                    # List all active leases (supports ?subnet=, ?mac=, ?hostname=, ?state=, ?limit=, ?offset=)
GET    /api/v1/leases/:ip                # Get single lease detail
DELETE /api/v1/leases/:ip                # Force-release a lease
GET    /api/v1/leases/export?format=csv  # Export leases as CSV
```

#### Reservations

```
GET    /api/v1/reservations              # List all reservations
POST   /api/v1/reservations              # Create reservation
PUT    /api/v1/reservations/:id          # Update reservation
DELETE /api/v1/reservations/:id          # Delete reservation
POST   /api/v1/reservations/import       # Bulk import from CSV
GET    /api/v1/reservations/export       # Export as CSV
```

#### Subnets & Pools

```
GET    /api/v1/subnets                   # List all subnets with pool utilisation
GET    /api/v1/subnets/:network          # Single subnet detail
GET    /api/v1/pools                     # All pools with utilisation stats
```

#### Conflicts

```
GET    /api/v1/conflicts                 # List all active conflicts
GET    /api/v1/conflicts/:ip             # Single conflict detail
DELETE /api/v1/conflicts/:ip             # Clear conflict, return IP to pool
POST   /api/v1/conflicts/:ip/exclude     # Permanently exclude IP from allocation
GET    /api/v1/conflicts/history         # Recently resolved conflicts
GET    /api/v1/conflicts/stats           # Conflict rates and patterns
```

#### Configuration

```
GET    /api/v1/config                    # Current running config (TOML as JSON object)
GET    /api/v1/config/raw                # Raw TOML text
PUT    /api/v1/config                    # Update config (validates, writes, reloads)
POST   /api/v1/config/validate           # Validate config without applying
GET    /api/v1/config/backups            # List config backups
GET    /api/v1/config/backups/:timestamp # Download a specific backup
```

#### Events

```
GET    /api/v1/events                    # Recent events (supports ?type=, ?limit=)
GET    /api/v1/events/stream             # WebSocket stream of live events
GET    /api/v1/hooks                     # Hook configuration and status
POST   /api/v1/hooks/test                # Fire a test event
```

#### HA

```
GET    /api/v1/ha/status                 # HA state, peer info, sync status
POST   /api/v1/ha/failover              # Manual failover trigger
GET    /api/v1/ha/sync/status            # Detailed sync metrics
```

#### System

```
GET    /api/v1/health                    # Health check (for load balancers)
GET    /api/v1/stats                     # Packet counters, error rates, uptime
GET    /metrics                          # Prometheus metrics endpoint
```

### Web UI Authentication

The embedded web UI shares the same auth as the API:

```toml
[api.auth]
auth_token = "changeme"         # for API/automation access

[[api.auth.users]]
username = "admin"
password_hash = "$2a$12$..."
role = "admin"                   # admin = full access, viewer = read-only

[[api.auth.users]]
username = "viewer"
password_hash = "$2a$12$..."
role = "viewer"
```

The web UI stores a session token in a cookie after login. API access uses Bearer token in the Authorization header. Both methods are supported simultaneously.

---

## Configuration Format (TOML)

### Example: `/etc/athena-dhcpd/config.toml`

```toml
# athena-dhcpd configuration

[server]
interface = "eth0"
# bind_address = "0.0.0.0"    # optional override
server_id = "10.0.0.1"         # option 54, usually the server's IP on the DHCP interface
log_level = "info"              # trace, debug, info, warn, error
lease_db = "/var/lib/athena-dhcpd/leases.db"
pid_file = "/run/athena-dhcpd.pid"

# Rate limiting / anti-starvation (RFC 5765)
[server.rate_limit]
enabled = true
max_discovers_per_second = 100
max_per_mac_per_second = 5

# --- Conflict Detection ---
[conflict_detection]
enabled = true
# Probe strategy: "sequential" (one at a time) or "parallel" (probe N candidates at once)
probe_strategy = "sequential"
# Timeout waiting for ARP/ICMP response
probe_timeout = "500ms"
# How many IPs to try before giving up and offering without probe
max_probes_per_discover = 3
# For parallel strategy: how many IPs to probe simultaneously
parallel_probe_count = 3
# How long a conflicted IP stays blocked before being eligible again
conflict_hold_time = "1h"
# After this many repeated conflicts on the same IP, mark it permanent (manual clear required)
max_conflict_count = 3
# Cache clear probes to avoid re-probing the same IP within this window
probe_cache_ttl = "10s"
# Send gratuitous ARP after successful lease assignment (local subnets only)
send_gratuitous_arp = true
# Use ICMP ping as fallback for remote/relayed subnets (when ARP not possible)
icmp_fallback = true
# Log level for probe activity (set to "debug" normally, "info" to see all probes)
probe_log_level = "debug"

# --- High Availability ---
[ha]
enabled = true
role = "primary"                # "primary" or "secondary"
peer_address = "10.0.0.2:6740"
listen_address = "0.0.0.0:6740"
heartbeat_interval = "1s"
failover_timeout = "10s"
sync_batch_size = 100

[ha.tls]
enabled = true
cert_file = "/etc/athena-dhcpd/tls/server.crt"
key_file = "/etc/athena-dhcpd/tls/server.key"
ca_file = "/etc/athena-dhcpd/tls/ca.crt"

# --- Event Hooks ---
[hooks]
event_buffer_size = 10000       # buffer before hooks, drop with warning if full
script_concurrency = 4          # max parallel script executions
script_timeout = "10s"

# Script hooks
[[hooks.script]]
name = "cmdb-update"
events = ["lease.ack", "lease.release", "lease.expire"]
command = "/etc/athena-dhcpd/hooks/update-cmdb.sh"
timeout = "5s"
subnets = ["10.10.0.0/24"]

[[hooks.script]]
name = "notify-noc"
events = ["ha.failover", "conflict.permanent"]
command = "/etc/athena-dhcpd/hooks/notify-noc.sh"

# Webhook hooks
[[hooks.webhook]]
name = "inventory-api"
events = ["lease.ack", "lease.release", "lease.expire"]
url = "https://inventory.athena.net.au/api/dhcp/events"
method = "POST"
headers = { "Authorization" = "Bearer secret123" }
timeout = "5s"
retries = 3
retry_backoff = "2s"
secret = "webhook-shared-secret"

[[hooks.webhook]]
name = "slack-alerts"
events = ["ha.failover", "lease.nak", "conflict.permanent"]
url = "https://hooks.slack.com/services/T00/B00/xxxx"
method = "POST"
template = "slack"

# --- Dynamic DNS ---
[ddns]
enabled = true
allow_client_fqdn = true
fallback_to_mac = true
ttl = 300
update_on_renew = false
conflict_policy = "overwrite"
use_dhcid = true

[ddns.forward]
zone = "office.athena.net.au"
method = "rfc2136"
server = "10.10.0.1:53"
tsig_name = "dhcp-update-key"
tsig_algorithm = "hmac-sha256"
tsig_secret = "base64encodedkey=="

[ddns.reverse]
zone = "10.10.in-addr.arpa"
method = "rfc2136"
server = "10.10.0.1:53"
tsig_name = "dhcp-update-key"
tsig_algorithm = "hmac-sha256"
tsig_secret = "base64encodedkey=="

# Per-subnet DDNS overrides
# [[ddns.zone_override]]
# subnet = "172.16.0.0/22"
# forward_zone = "wireless.athena.net.au"
# reverse_zone = "16.172.in-addr.arpa"
# method = "powerdns_api"
# server = "http://10.10.0.2:8081"
# api_key = "pdns-api-key-here"

# --- Subnet Definitions ---
[[subnet]]
network = "10.10.0.0/24"
routers = ["10.10.0.1"]
dns_servers = ["1.1.1.1", "8.8.8.8"]
domain_name = "office.athena.net.au"
lease_time = "8h"
renewal_time = "4h"
rebind_time = "7h"
ntp_servers = ["10.10.0.1"]

  [[subnet.pool]]
  range_start = "10.10.0.100"
  range_end = "10.10.0.200"

  [[subnet.pool]]
  range_start = "10.10.0.201"
  range_end = "10.10.0.250"
  lease_time = "1h"
  match_circuit_id = "eth0/1/*"
  match_vendor_class = "Cisco*"

[[subnet.reservation]]
mac = "aa:bb:cc:dd:ee:f0"
ip = "10.10.0.10"
hostname = "switch-core"
dns_servers = ["10.10.0.1"]
ddns_hostname = "core-switch"

[[subnet.reservation]]
identifier = "01:aa:bb:cc:dd:ee:f1"
ip = "10.10.0.11"
hostname = "ap-lobby"

[[subnet]]
network = "172.16.0.0/22"
routers = ["172.16.0.1"]
dns_servers = ["1.1.1.1", "9.9.9.9"]
domain_name = "wireless.athena.net.au"
lease_time = "2h"

  [[subnet.pool]]
  range_start = "172.16.0.10"
  range_end = "172.16.3.250"

  [[subnet.option]]
  code = 43
  type = "hex"
  value = "010a0a0a0a01"

  [[subnet.option]]
  code = 150
  type = "ip_list"
  value = ["10.0.0.5", "10.0.0.6"]

# --- Global Options ---
[defaults]
lease_time = "12h"
renewal_time = "6h"
rebind_time = "10h30m"
dns_servers = ["1.1.1.1", "1.0.0.1"]
domain_name = "athena.net.au"

# --- Web Interface & API ---
[api]
enabled = true
listen = "0.0.0.0:8067"
web_ui = true

[api.auth]
auth_token = "changeme"

[[api.auth.users]]
username = "admin"
password_hash = "$2a$12$LJ3m4ys3Lz..."
role = "admin"

[[api.auth.users]]
username = "viewer"
password_hash = "$2a$12$xK9p2wR1Qz..."
role = "viewer"

[api.tls]
enabled = false
cert_file = "/etc/athena-dhcpd/tls/web.crt"
key_file = "/etc/athena-dhcpd/tls/web.key"

[api.session]
cookie_name = "athena_session"
expiry = "24h"
secure = false
```

### Config Hierarchy (option inheritance)

```
defaults → subnet → pool → reservation
```

More specific scopes override less specific ones. The lease engine resolves the final option set at allocation time by merging from left to right.

---

## Project Structure

```
athena-dhcpd/
├── .windsurfrules
├── .claude/
│   └── project-plan.md
├── cmd/
│   └── athena-dhcpd/
│       └── main.go             # Entry point, signal handling, CLI flags
├── internal/
│   ├── config/
│   │   ├── config.go           # TOML parsing, validation, hot-reload (SIGHUP)
│   │   ├── config_test.go
│   │   └── defaults.go
│   ├── dhcp/
│   │   ├── server.go           # Core DHCP server (listen, dispatch)
│   │   ├── handler.go          # DORA message handlers
│   │   ├── packet.go           # DHCPv4 packet encode/decode
│   │   ├── packet_test.go
│   │   ├── options.go          # Option registry, encode/decode
│   │   ├── options_test.go
│   │   ├── options_registry.go # All 254 option definitions
│   │   └── relay.go            # Option 82 handling, giaddr logic
│   ├── lease/
│   │   ├── manager.go          # Lease allocation, renewal, expiry
│   │   ├── manager_test.go
│   │   ├── store.go            # BoltDB storage interface
│   │   ├── store_test.go
│   │   ├── types.go            # Lease struct, states
│   │   └── gc.go               # Expired lease garbage collection
│   ├── pool/
│   │   ├── allocator.go        # IP allocation from ranges
│   │   ├── allocator_test.go
│   │   └── matcher.go          # Pool matching (opt82, vendor class)
│   ├── conflict/
│   │   ├── detector.go         # Conflict detection coordinator
│   │   ├── detector_test.go
│   │   ├── arp.go              # Raw ARP probe implementation
│   │   ├── arp_test.go
│   │   ├── icmp.go             # ICMP echo probe implementation
│   │   ├── icmp_test.go
│   │   ├── gratuitous.go       # Gratuitous ARP sender
│   │   ├── table.go            # Conflict table (BoltDB + in-memory)
│   │   ├── table_test.go
│   │   └── cache.go            # Probe result cache (TTL-based)
│   ├── ha/
│   │   ├── peer.go             # Peer connection management
│   │   ├── sync.go             # Lease sync protocol (send/recv)
│   │   ├── heartbeat.go        # Heartbeat and failure detection
│   │   ├── failover.go         # Failover state machine
│   │   ├── protocol.go         # Wire protocol definition
│   │   └── ha_test.go
│   ├── events/
│   │   ├── bus.go              # Event bus (buffered channel, fan-out)
│   │   ├── bus_test.go
│   │   ├── types.go            # Event types and payload structs
│   │   ├── hooks.go            # Hook dispatcher (script + webhook)
│   │   ├── hooks_test.go
│   │   ├── script_runner.go    # Script execution with timeout/env
│   │   ├── webhook_sender.go   # HTTP webhook with retry
│   │   └── templates.go        # Built-in webhook templates (Slack, Teams)
│   ├── ddns/
│   │   ├── manager.go          # DDNS lifecycle (subscribe to events, dispatch updates)
│   │   ├── manager_test.go
│   │   ├── rfc2136.go          # RFC 2136 DNS UPDATE client
│   │   ├── rfc2136_test.go
│   │   ├── tsig.go             # TSIG signing (RFC 2845)
│   │   ├── powerdns.go         # PowerDNS HTTP API client
│   │   ├── technitium.go       # Technitium DNS HTTP API client
│   │   ├── dhcid.go            # DHCID record generation (RFC 4701)
│   │   ├── fqdn.go             # FQDN construction logic
│   │   └── fqdn_test.go
│   ├── api/
│   │   ├── server.go           # HTTP server, router, middleware
│   │   ├── auth.go             # Token auth, session auth, bcrypt
│   │   ├── handlers_leases.go
│   │   ├── handlers_reservations.go
│   │   ├── handlers_config.go
│   │   ├── handlers_conflicts.go  # Conflict table CRUD + stats
│   │   ├── handlers_events.go
│   │   ├── handlers_ha.go
│   │   ├── handlers_stats.go
│   │   └── websocket.go
│   ├── webui/
│   │   ├── embed.go            # go:embed directive, static file serving
│   │   └── spa.go              # SPA fallback handler (all routes → index.html)
│   └── logging/
│       └── logger.go
├── pkg/
│   └── dhcpv4/
│       ├── constants.go
│       └── encoding.go
├── web/                        # Frontend source (React + TypeScript + Tailwind)
│   ├── src/
│   │   ├── App.tsx
│   │   ├── main.tsx
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx
│   │   │   ├── Leases.tsx
│   │   │   ├── Reservations.tsx
│   │   │   ├── Subnets.tsx
│   │   │   ├── Config.tsx
│   │   │   ├── Events.tsx
│   │   │   ├── Conflicts.tsx
│   │   │   └── HAStatus.tsx
│   │   ├── components/
│   │   │   ├── Layout.tsx
│   │   │   ├── LeaseTable.tsx
│   │   │   ├── PoolBar.tsx
│   │   │   ├── SubnetCard.tsx
│   │   │   ├── EventLog.tsx
│   │   │   ├── ConfigEditor.tsx
│   │   │   ├── ConflictTable.tsx
│   │   │   ├── ReservationForm.tsx
│   │   │   ├── HAIndicator.tsx
│   │   │   ├── LoginForm.tsx
│   │   │   └── ConfirmDialog.tsx
│   │   ├── api/
│   │   │   └── client.ts
│   │   ├── hooks/
│   │   │   ├── useLeases.ts
│   │   │   ├── useEventStream.ts
│   │   │   └── useAuth.ts
│   │   └── types/
│   │       └── index.ts
│   ├── index.html
│   ├── tailwind.config.js
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json
├── configs/
│   └── example.toml
├── scripts/
│   ├── install.sh
│   └── test-client.sh
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

---

## Implementation Phases

### Phase 1: Core DHCP Engine + Conflict Detection

**Goal:** A working single-node DHCP server that handles DORA correctly with IP conflict detection before every offer.

1. Set up Go module, project structure, Makefile
2. Implement TOML config parsing with validation
3. Implement DHCPv4 packet encode/decode (RFC 2131)
4. Implement option registry with all standard options (RFC 2132)
5. Implement lease manager with BoltDB backend
6. Implement IP pool allocator with range support
7. **Implement ARP probe module** — raw AF_PACKET socket, ARP request/reply, timeout
8. **Implement ICMP echo probe module** — raw ICMP socket, echo request/reply, timeout
9. **Implement conflict detector coordinator** — selects ARP vs ICMP based on subnet locality, manages probing strategy (sequential/parallel), handles timeouts
10. **Implement conflict table** — BoltDB bucket + in-memory cache, hold timer, max conflict threshold, permanent flagging
11. **Implement probe cache** — TTL-based cache of recently-probed-clear IPs
12. **Integrate conflict detection into DORA flow** — probe after IP selection, before OFFER; retry with next IP on conflict
13. **Implement DHCPDECLINE handler** — mark IP as conflicted on client-side detection
14. **Implement gratuitous ARP sender** — optional post-ACK announcement for local subnets
15. Implement DORA handlers (Discover → Probe → Offer → Request → Ack → Gratuitous ARP)
16. Implement INIT-REBOOT, renewing, rebinding states
17. Implement DHCPRELEASE handling
18. Implement lease expiry GC goroutine
19. Add reservation (static) host support
20. Implement event bus (internal/events) — including conflict.* event types
21. Write comprehensive tests:
    - Packet handling and lease logic
    - ARP probe with mock raw socket
    - ICMP probe with mock
    - Conflict table lifecycle (detect → hold → resolve / permanent)
    - Probe cache TTL behaviour
    - DHCPDECLINE → conflict marking
    - Full DORA with probe integration (mock ARP clear, mock ARP conflict + retry)

**Exit criteria:** Server can allocate IPs to real DHCP clients on a test network. ARP probes fire before every offer (visible in debug logs). Conflicted IPs are skipped and retried. DHCPDECLINE marks IPs in conflict table. Leases survive restart. Event bus fires events for all lease state changes and conflict events. `CAP_NET_RAW` requirement documented.

### Phase 2: Options, Relay & Event Hooks

**Goal:** Full option support, relay agent handling, pool matching, and working event hooks.

1. Implement relay agent (option 82) parsing and forwarding (RFC 3046)
2. Implement classless static routes option 121 (RFC 3442)
3. Implement pool matching on circuit-id, remote-id, vendor class
4. Implement subnet selection via giaddr and option 118 (RFC 3011)
5. Implement link selection sub-option (RFC 3527)
6. Implement client FQDN option 81 (RFC 4702)
7. Implement vendor-identifying options 124/125 (RFC 3925)
8. Implement custom option passthrough (`[[subnet.option]]` config)
9. Implement BOOTP compatibility
10. Add rate limiting / anti-starvation (RFC 5765)
11. **Verify ICMP fallback works for relayed subnets** — ARP can't cross L3, must use ICMP
12. Implement script hook runner (internal/events/script_runner.go)
13. Implement webhook sender with retry logic
14. Implement built-in webhook templates (Slack, Teams)
15. Write tests for hooks — mock script execution, mock HTTP for webhooks
16. Write tests for conflict detection via relay (ICMP path)

**Exit criteria:** Server works behind relay agents, handles option 82 correctly, supports all major DHCP options, matches pools on relay/vendor criteria. ICMP conflict probes work for relayed subnets. Script and webhook hooks fire correctly on all configured events including conflict events.

### Phase 3: Dynamic DNS

**Goal:** Automatic DNS registration/deregistration for all leases.

1. Implement FQDN construction logic (option 81, hostname+domain, MAC fallback)
2. Implement RFC 2136 DNS UPDATE packet builder using miekg/dns
3. Implement TSIG signing (HMAC-MD5, HMAC-SHA256, HMAC-SHA512)
4. Implement forward zone (A record) updates — add on ack, remove on release/expire
5. Implement reverse zone (PTR record) updates
6. Implement DHCID record support (RFC 4701) for conflict detection
7. Implement conflict resolution policies (overwrite/skip/append)
8. Implement PowerDNS HTTP API client
9. Implement Technitium DNS HTTP API client
10. Subscribe DDNS manager to event bus (lease.ack, lease.renew, lease.release, lease.expire)
11. Implement per-subnet zone overrides
12. Write integration tests against a test BIND instance (or mock DNS)

**Exit criteria:** New leases automatically appear in DNS (forward + reverse), released/expired leases are cleaned from DNS, TSIG auth works, PowerDNS and Technitium API integrations tested.

### Phase 4: High Availability

**Goal:** Two-node HA with automatic failover.

1. Define sync wire protocol (protobuf or msgpack over TCP)
2. Implement peer connection manager with reconnect logic
3. Implement heartbeat mechanism (UDP, configurable interval)
4. Implement lease sync — new/updated/released/expired events
5. **Implement conflict table sync** — conflict records replicated to standby node
6. Implement full lease database sync on peer startup (bulk transfer)
7. Implement failover state machine:
   - `PARTNER_UP` — both nodes communicating, primary serves
   - `PARTNER_DOWN` — peer unreachable, start failover timer
   - `ACTIVE` — this node is serving (post-failover or primary)
   - `STANDBY` — this node is passive, receiving sync only
   - `RECOVERY` — peer reconnected, reconciling lease state
8. Implement split-brain protection
9. Implement lease conflict resolution (last-update-wins with vector clock)
10. Add TLS support for peer communication
11. Fire HA events (ha.failover, ha.sync_complete) through the event bus
12. Write integration tests with two-node simulation

**Exit criteria:** Two nodes stay in sync (leases AND conflict table), failover completes within configured timeout, no duplicate allocations, clean re-sync on peer reconnect. HA events trigger configured hooks.

### Phase 5: Web Interface, API & Hardening

**Goal:** Full web management interface and production readiness.

1. Implement HTTP API server with router and middleware
2. Implement authentication (bearer token + session-based for web UI)
3. Implement all API endpoints (leases, reservations, **conflicts**, config, events, HA, stats)
4. Implement WebSocket endpoint for live event streaming
5. Implement config write-back with validation, diff, and backup
6. Scaffold React+TypeScript+Tailwind frontend project in `web/`
7. Implement Dashboard page (pool utilisation, counters, **conflict count**, HA status, recent events)
8. Implement Leases page (table with search, filter, sort, export, actions)
9. Implement Reservations page (CRUD, import/export CSV, validation)
10. Implement Subnets page (visual overview, pool details)
11. Implement Config page (syntax-highlighted viewer, editable sections, diff, apply)
12. Implement Events page (live WebSocket log, hook status, test hook)
13. **Implement Conflicts page** (active conflicts table, clear/exclude actions, history, stats)
14. Implement HA Status page (peer info, sync metrics, manual failover)
15. Implement Login page and auth flow
16. Set up `go:embed` for static assets, SPA fallback handler
17. Add Makefile targets: `build-web`, `build` (depends on web), `dev`
18. Add Prometheus metrics endpoint (`/metrics`) — include conflict detection metrics
19. Implement config hot-reload on SIGHUP
20. Implement graceful shutdown (finish in-flight, sync final state, cleanup DNS)
21. Add systemd service file and install script — **document CAP_NET_RAW requirement**
22. Write Dockerfile (multi-stage: Node build → Go build → minimal runtime) — **set capabilities**
23. Security hardening: drop privileges, seccomp, read-only rootfs support
24. Write README with deployment guide

**Exit criteria:** Server is deployable in production. Web UI provides full visibility and management including conflict inspection. Config changes can be made through the UI. Observable via Prometheus. Handles operational lifecycle cleanly.

---

## Key Dependencies (Go Modules)

```
go.etcd.io/bbolt                         — Embedded key-value store (BoltDB)
github.com/BurntSushi/toml               — TOML config parser
github.com/insomniacslk/dhcp             — DHCPv4 packet library (evaluate, or roll own)
github.com/miekg/dns                     — DNS library (for RFC 2136 updates + TSIG)
github.com/mdlayher/arp                  — ARP client library (evaluate vs raw AF_PACKET)
github.com/mdlayher/raw                  — Raw socket abstraction (if not using arp lib)
google.golang.org/protobuf               — HA sync protocol encoding
github.com/prometheus/client_golang       — Metrics
github.com/gorilla/websocket             — WebSocket for live event stream
golang.org/x/crypto/bcrypt               — Password hashing for web UI auth
golang.org/x/net/icmp                    — ICMP packet handling
golang.org/x/net/ipv4                    — IPv4 raw socket support
```

**Decision point:** For ARP probing, evaluate `github.com/mdlayher/arp` — it provides a clean Go-native ARP client over raw sockets. If it's too high-level or doesn't support the probe patterns we need, implement directly on `AF_PACKET` via `syscall`. The `mdlayher/raw` package can help abstract the raw socket if going that route.

**Decision point:** For ICMP, use `golang.org/x/net/icmp` which is the standard Go extended library for ICMP. It handles packet construction and parsing cleanly.

**Frontend (web/):**

```
react, react-dom, react-router-dom       — SPA framework
typescript                                — Type safety
tailwindcss                               — Styling
@tanstack/react-table                     — Lease/reservation/conflict tables
recharts                                  — Dashboard charts
vite                                      — Build tooling
```

---

## Coding Standards & Rules

### General

- Go 1.22+ minimum
- No global mutable state — dependency injection via structs
- All public functions must have godoc comments
- Errors must be wrapped with context: `fmt.Errorf("allocating IP for %s: %w", mac, err)`
- No panics in library code — return errors
- Use `context.Context` for cancellation throughout
- Use `log/slog` for structured logging (no third-party logging libs)

### Conflict Detection

- The conflict detector MUST be called before every DHCPOFFER for dynamically allocated IPs
- Reservations (static leases) are exempt from probing by default (configurable)
- ARP raw socket is opened once at startup and shared across probes — NOT opened per-probe
- ICMP socket similarly opened once and shared
- Probe timeout is enforced via context cancellation — never hang waiting for a reply
- Conflict table is persisted in BoltDB AND cached in memory for O(1) lookup
- Conflict table is synced to HA peer along with lease data
- Probe cache uses sync.Map or similar concurrent-safe structure
- ALL probe results (hit or miss) are logged at debug level with timing
- Conflict events fire through the event bus — hooks can alert on conflicts
- The server MUST still function (with reduced safety) if raw socket creation fails (e.g., missing CAP_NET_RAW) — log a loud warning at startup, skip probes, proceed with allocation

### Event Hooks

- Event bus MUST be non-blocking — use buffered channel, drop events if full
- Script execution uses `os/exec` — this is the ONLY permitted use of os/exec in the project
- Scripts run in a goroutine pool with configurable concurrency limit
- Script timeout is enforced via context cancellation — always kill on timeout
- Hook failures NEVER propagate to DHCP processing — log and move on
- Webhook HTTP client must use a connection pool with reasonable timeouts

### DDNS

- DDNS updates are asynchronous to DHCP responses — never block an ACK waiting for DNS
- Failed DNS updates should be retried with backoff (configurable)
- DNS cleanup on lease release/expire is best-effort — log failures, don't block
- TSIG secrets in config should be flagged as sensitive in log output (redacted)

### Web UI

- Frontend is built separately and embedded via `go:embed` — no Node.js at runtime
- SPA routing: all non-API, non-metrics paths serve index.html
- API responses are always JSON with consistent error format: `{"error": "message", "code": "ERROR_CODE"}`
- WebSocket connections should have a ping/pong keepalive
- Config write-back must be atomic (write to temp file, rename) and create a backup first
- Web UI must work without JavaScript for basic health checks (the /api/v1/health endpoint)

### Testing

- Table-driven tests for packet encoding/decoding and option serialisation
- Integration tests using real UDP sockets on loopback
- **ARP and ICMP probe tests using mock raw sockets** — test both hit and miss paths
- **Conflict table tests**: full lifecycle (detect, hold, resolve, permanent, manual clear)
- **Probe cache tests**: TTL expiry, invalidation on DECLINE
- HA tests using goroutine-based node simulation
- DNS update tests against mock DNS server (using miekg/dns test utilities)
- Hook tests with mock script execution and mock HTTP server
- Minimum 80% coverage on `internal/dhcp`, `internal/lease`, `internal/ha`, `internal/ddns`, `internal/conflict`
- Run `go vet`, `staticcheck`, and `golangci-lint` in CI

### Performance

- Zero-allocation packet path where possible (pre-allocated buffers)
- Lease lookups must be O(1) by IP and by client-id/MAC
- Pool allocation should avoid linear scans (use bitmap or free-list)
- BoltDB writes batched where possible
- Event bus should handle 10k+ events/sec without backpressure on DHCP
- **ARP probe adds max `probe_timeout` latency per candidate IP** — document this tradeoff
- **Probe cache eliminates repeated probes for the same IP within TTL window**
- **Parallel probe strategy caps worst-case latency at one timeout regardless of conflicts**

### Security

- Validate ALL packet fields — malformed packets must be dropped with a log, never crash
- Rate limit per-MAC to prevent starvation attacks
- HA TLS is strongly recommended in production
- API auth token required if API is exposed beyond localhost
- Web UI passwords stored as bcrypt hashes only — never plaintext
- TSIG secrets and API keys redacted in log output and config API responses
- Config backups should not be world-readable (0600 permissions)
- **CAP_NET_RAW capability required for ARP and ICMP probes** — document in README, systemd unit, and Dockerfile

### Config

- Config must be validated fully at startup — fail fast on bad config
- Duration fields accept Go-style strings: `"30m"`, `"8h"`, `"1h30m"`
- IP validation on all address fields
- CIDR validation on network fields
- Warn on overlapping pools or subnets
- Config write-back from API/UI must validate before writing and create automatic backup

---

## HA Sync Protocol Detail

### Wire Format

Each sync message is a length-prefixed frame over TCP:

```
[4 bytes: payload length (big-endian uint32)]
[1 byte:  message type]
[N bytes: payload (protobuf or msgpack)]
```

### Message Types

| Type | Name             | Direction        | Description                           |
| ---- | ---------------- | ---------------- | ------------------------------------- |
| 0x01 | HEARTBEAT        | Bidirectional    | Keepalive with state info             |
| 0x02 | LEASE_UPDATE     | Active → Standby | Single lease created/renewed/released |
| 0x03 | LEASE_BULK_START | Active → Standby | Beginning full sync                   |
| 0x04 | LEASE_BULK_DATA  | Active → Standby | Batch of leases                       |
| 0x05 | LEASE_BULK_END   | Active → Standby | Full sync complete                    |
| 0x06 | FAILOVER_CLAIM   | Either           | Asserting active role                 |
| 0x07 | FAILOVER_ACK     | Either           | Acknowledging peer's claim            |
| 0x08 | STATE_REQUEST    | Either           | Request full lease dump               |
| 0x09 | CONFLICT_UPDATE  | Active → Standby | Conflict table entry added/resolved   |
| 0x0A | CONFLICT_BULK    | Active → Standby | Full conflict table sync              |

### Lease Update Payload

```json
{
  "ip": "10.10.0.105",
  "mac": "aa:bb:cc:dd:ee:01",
  "client_id": "...",
  "hostname": "device-1",
  "fqdn": "device-1.office.athena.net.au",
  "start": 1708500000,
  "expiry": 1708528800,
  "state": "active",
  "subnet": "10.10.0.0/24",
  "last_updated": 1708500000,
  "update_seq": 42
}
```

---

## Lease Storage Schema (BoltDB)

### Buckets

```
leases/
  key: IP address (string "10.10.0.105")
  value: JSON-encoded Lease struct

index_mac/
  key: MAC address (string "aa:bb:cc:dd:ee:01")
  value: IP address

index_client_id/
  key: client identifier (hex string)
  value: IP address

index_hostname/
  key: hostname (string, lowercase)
  value: IP address (for DDNS conflict checking)

conflicts/
  key: IP address (string "10.10.0.105")
  value: JSON-encoded Conflict struct
    {
      "ip": "10.10.0.105",
      "detected_at": "2025-02-21T14:30:00Z",
      "detection_method": "arp_probe",
      "responder_mac": "aa:bb:cc:11:22:33",
      "hold_until": "2025-02-21T15:30:00Z",
      "subnet": "10.10.0.0/24",
      "probe_count": 1,
      "permanent": false
    }

excluded_ips/
  key: IP address (string "10.10.0.42")
  value: JSON-encoded exclusion record (reason, who excluded, when)

meta/
  key: "sequence"
  value: uint64 — monotonic update sequence counter
  key: "last_sync"
  value: unix timestamp of last successful peer sync

event_log/
  key: timestamp-based ID (for recent event retrieval by web UI)
  value: JSON-encoded event (ring buffer, configurable max size)
```

---

## Prometheus Metrics (Conflict-Related)

In addition to standard DHCP metrics, the following conflict-specific metrics are exported:

```
athena_dhcpd_conflict_probes_total{method="arp|icmp", result="clear|conflict|timeout|error"}
athena_dhcpd_conflict_probe_duration_seconds{method="arp|icmp"}
athena_dhcpd_conflicts_active{subnet="..."}
athena_dhcpd_conflicts_permanent{subnet="..."}
athena_dhcpd_conflicts_resolved_total{method="hold_expired|manual_clear"}
athena_dhcpd_conflict_declines_total{subnet="..."}
athena_dhcpd_probe_cache_hits_total
athena_dhcpd_probe_cache_misses_total
athena_dhcpd_event_buffer_drops_total
```

---

## Success Criteria

1. **Functional:** Allocates IPs correctly to real DHCP clients (tested with `dhclient`, ISC DHCP relay, MikroTik DHCP relay)
2. **Compliant:** Passes `dhcptest` (or similar) validation tool for RFC 2131/2132 compliance
3. **Conflict-free:** ARP probes detect in-use IPs before offering; DHCPDECLINE marks IPs correctly; conflicted IPs are held and retried; permanent conflicts flagged for manual resolution
4. **Resilient:** Survives process restart with all leases and conflict table intact
5. **HA:** Failover completes within configured timeout, no duplicate allocations, conflict table synced, clean re-sync
6. **DDNS:** Leases automatically registered in DNS (A + PTR), cleaned on release/expire, TSIG auth works
7. **Hooks:** Script and webhook hooks fire on configured events (including conflict events), don't impact DHCP performance
8. **Web UI:** Dashboard shows real-time status including conflicts, leases browseable and searchable, reservations manageable via UI, conflict table inspectable and clearable, config editable with validation
9. **Observable:** Prometheus metrics (including conflict probe stats), structured logs, health API endpoint
10. **Performant:** Handles 1000+ DORA cycles/second on modest hardware (with probe cache warm); worst-case probe latency documented
11. **Deployable:** Single static binary (with embedded web UI), systemd service with CAP_NET_RAW, Docker image with capabilities set