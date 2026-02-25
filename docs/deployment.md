# Deployment

athena-dhcpd ships as a single binary. no runtime dependencies, no interpreters, no package managers on the target machine. copy the binary, write a config, run it

## building

```bash
# just the Go binary (no web UI)
go build -o athena-dhcpd ./cmd/athena-dhcpd

# full build including web UI
make build

# cross-compile for linux/amd64 from wherever
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o athena-dhcpd ./cmd/athena-dhcpd
```

requires Go 1.22+ and Node.js 18+ (Node is only needed for building the frontend, not at runtime)

## capabilities

DHCP uses port 67 (privileged) and conflict detection uses raw sockets. you need some Linux capabilities:

| Capability | Why |
|-----------|-----|
| `CAP_NET_BIND_SERVICE` | Bind to port 67 |
| `CAP_NET_RAW` | Raw sockets for ARP/ICMP conflict detection |

three ways to handle this:

### option 1: setcap on the binary (simple)
```bash
sudo setcap 'cap_net_raw,cap_net_bind_service+ep' /usr/local/bin/athena-dhcpd
```

note: setcap gets wiped if you replace the binary. re-run after updates

### option 2: systemd AmbientCapabilities (recommended)
the included service file handles this. see systemd section below

### option 3: run as root
```bash
sudo ./athena-dhcpd -config /etc/athena-dhcpd/config.toml
```
works but you know... root

if the server can't get CAP_NET_RAW, conflict detection gets disabled automatically. DHCP still works, you just lose the probing safety net. it logs a big warning about it

## directory setup

```bash
# config directory
sudo mkdir -p /etc/athena-dhcpd
sudo cp configs/example.toml /etc/athena-dhcpd/config.toml
sudo chmod 600 /etc/athena-dhcpd/config.toml   # has secrets

# data directory (lease database)
sudo mkdir -p /var/lib/athena-dhcpd

# if running as a dedicated user
sudo useradd -r -s /sbin/nologin athena-dhcpd
sudo chown athena-dhcpd:athena-dhcpd /var/lib/athena-dhcpd
sudo chown athena-dhcpd:athena-dhcpd /etc/athena-dhcpd/config.toml
```

## systemd

theres a ready-to-go service file at `deploy/athena-dhcpd.service`

```bash
sudo cp deploy/athena-dhcpd.service /etc/systemd/system/
sudo cp build/athena-dhcpd /usr/local/bin/
sudo systemctl daemon-reload
sudo systemctl enable --now athena-dhcpd
```

### whats in the service file

```ini
[Service]
Type=simple
ExecStart=/usr/local/bin/athena-dhcpd -config /etc/athena-dhcpd/config.toml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s

User=athena-dhcpd
Group=athena-dhcpd

# Capabilities
AmbientCapabilities=CAP_NET_RAW CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_BIND_SERVICE

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/athena-dhcpd /run/athena-dhcpd
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RestrictRealtime=yes
```

the security hardening directives lock down the process pretty tight:
- `ProtectSystem=strict` — filesystem is read-only except for explicitly allowed paths
- `ProtectHome=yes` — can't access home directories
- `NoNewPrivileges=yes` — can't escalate privileges
- `PrivateTmp=yes` — gets its own /tmp
- `MemoryDenyWriteExecute=yes` — prevents JIT-style attacks (we're Go, we don't need W+X pages)

### config reload without restart

```bash
sudo systemctl reload athena-dhcpd
```

this sends SIGHUP, which triggers config hot-reload. pools, rate limits, and most settings get updated in place. leases and connections are preserved

### checking status

```bash
sudo systemctl status athena-dhcpd
sudo journalctl -u athena-dhcpd -f    # follow logs
```

## Docker

```bash
docker build -t athena-dhcpd .
```

### running

```bash
docker run -d \
  --name athena-dhcpd \
  --cap-add=NET_RAW \
  --cap-add=NET_BIND_SERVICE \
  --network=host \
  -v /etc/athena-dhcpd:/etc/athena-dhcpd \
  -v /var/lib/athena-dhcpd:/var/lib/athena-dhcpd \
  athena-dhcpd
```

**`--network=host` is required.** DHCP uses broadcast packets and needs to be on the actual network. you cannot use bridge networking for a DHCP server. this is a DHCP protocol thing, not an athena limitation. every DHCP server in Docker needs host networking

`--cap-add=NET_RAW` and `--cap-add=NET_BIND_SERVICE` are needed for the same reasons as bare metal

### docker compose

```yaml
services:
  athena-dhcpd:
    build: .
    network_mode: host
    cap_add:
      - NET_RAW
      - NET_BIND_SERVICE
    volumes:
      - /etc/athena-dhcpd:/etc/athena-dhcpd
      - dhcp-data:/var/lib/athena-dhcpd
    restart: unless-stopped

volumes:
  dhcp-data:
```

### whats in the Dockerfile

multi-stage build:
1. **builder** — golang:1.22-alpine, builds a static binary with CGO_ENABLED=0
2. **runtime** — alpine:3.19, minimal image with just ca-certificates and tzdata

the binary runs as a non-root `athena` user. the image is about 15MB

## ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 67 | UDP | DHCP server |
| 8067 | TCP | HA peer communication (if enabled) |
| 8080 | TCP | API + Web UI (configurable via `api.listen`) |

## firewall rules

if you're running a firewall (you should be), you need:

```bash
# DHCP (always)
sudo ufw allow 67/udp

# API/Web UI (if using)
sudo ufw allow 8080/tcp

# HA peer (if using, only from peer IP)
sudo ufw allow from 192.168.1.2 to any port 8067 proto tcp
```

or with iptables:
```bash
iptables -A INPUT -p udp --dport 67 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
iptables -A INPUT -p tcp --dport 8067 -s 192.168.1.2 -j ACCEPT
```

## security checklist

- [ ] config file has 0600 permissions (contains secrets)
- [ ] running as a dedicated non-root user
- [ ] capabilities set (not running as root)
- [ ] API auth token set in production
- [ ] web UI passwords are bcrypt hashes (not plaintext)
- [ ] API TLS enabled if exposed to network
- [ ] HA TLS enabled if peers communicate over untrusted network
- [ ] firewall restricts access to API and HA ports
- [ ] lease database directory only writable by the service user
- [ ] TSIG secrets and API keys stored securely

## backup strategy

### lease database
BoltDB file at `/var/lib/athena-dhcpd/leases.db`. you can copy it while the server is running — BoltDB supports concurrent reads. or just stop the server, copy, start again

leases have expiry times so even if you lose the database, clients will just re-request their IPs when the lease expires. its not the end of the world, but its not great either

### config
config backups are created automatically by the API when you update config via the web UI. they're stored alongside the config file with timestamps:

```
config.toml.bak.20240123T143022
config.toml.bak.20240122T091500
```

you should also back these up to wherever you back things up to. version control is a good idea for config files
