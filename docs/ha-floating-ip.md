# HA with Floating IP for DNS Proxy

when running athena-dhcpd in HA mode, DHCP failover is straightforward — clients broadcast DHCPDISCOVER and whichever node is active will respond. but DNS is different. clients are configured with a specific IP address for their DNS server, so if that server dies, DNS breaks even though DHCP keeps working

the fix is a **floating IP** (also called a virtual IP or VIP) that moves between the two nodes. whichever node is active owns the VIP. when failover happens, the VIP moves to the new active node. clients always point at the VIP and never know the difference

## the problem

say you have two nodes:
- **node A** (primary): `10.0.0.1`
- **node B** (secondary): `10.0.0.2`

both run athena-dhcpd with HA and the DNS proxy enabled. you hand out `dns_servers = ["10.0.0.1"]` to clients via DHCP

node A crashes. node B takes over DHCP — great, new clients get IPs. but every existing client still has `10.0.0.1` as their DNS server. DNS is broken for them until node A comes back or they renew their lease and get node B's address

thats not good enough

## the solution: floating IP with keepalived

use a third IP address — the VIP — that floats between the two nodes. clients always use the VIP for DNS. keepalived handles moving the VIP automatically based on which athena-dhcpd node is active

### network layout

| Host | Physical IP | Role |
|------|------------|------|
| node A | `10.0.0.1` | primary |
| node B | `10.0.0.2` | secondary |
| VIP | `10.0.0.3` | floating (active node owns this) |

### install keepalived

```bash
# debian/ubuntu
sudo apt install keepalived

# rhel/centos/rocky
sudo dnf install keepalived
```

### athena-dhcpd health check script

create a script that keepalived uses to check if the local athena-dhcpd instance is active and healthy. keepalived will only assign the VIP to a node where this script exits 0

```bash
#!/bin/bash
# /etc/keepalived/check_athena.sh

# check if athena-dhcpd is running
if ! systemctl is-active --quiet athena-dhcpd; then
    exit 1
fi

# check if the API responds and reports healthy
STATUS=$(curl -sf --max-time 2 http://127.0.0.1:8067/api/v1/health | grep -o '"status":"ok"')
if [ -z "$STATUS" ]; then
    exit 1
fi

# check if this node is the active HA node
HA_STATE=$(curl -sf --max-time 2 http://127.0.0.1:8067/api/v1/ha/status | grep -o '"state":"ACTIVE"')
if [ -z "$HA_STATE" ]; then
    exit 1
fi

exit 0
```

```bash
sudo chmod +x /etc/keepalived/check_athena.sh
```

### keepalived config — node A (primary)

```conf
# /etc/keepalived/keepalived.conf on node A

vrrp_script check_athena {
    script "/etc/keepalived/check_athena.sh"
    interval 2          # check every 2 seconds
    weight 20           # add 20 to priority when check passes
    fall 3              # require 3 failures before marking down
    rise 2              # require 2 successes before marking up
}

vrrp_instance ATHENA_DNS {
    state MASTER
    interface eth0              # your network interface
    virtual_router_id 51       # must match on both nodes (1-255)
    priority 100               # higher = preferred (primary gets 100)

    advert_int 1               # VRRP advertisement interval

    authentication {
        auth_type PASS
        auth_pass somesecret   # must match on both nodes
    }

    virtual_ipaddress {
        10.0.0.3/24            # the floating IP
    }

    track_script {
        check_athena
    }

    # optional: notify script when VIP moves
    notify_master "/etc/keepalived/notify.sh MASTER"
    notify_backup "/etc/keepalived/notify.sh BACKUP"
    notify_fault  "/etc/keepalived/notify.sh FAULT"
}
```

### keepalived config — node B (secondary)

same thing but with lower priority and `state BACKUP`:

```conf
# /etc/keepalived/keepalived.conf on node B

vrrp_script check_athena {
    script "/etc/keepalived/check_athena.sh"
    interval 2
    weight 20
    fall 3
    rise 2
}

vrrp_instance ATHENA_DNS {
    state BACKUP
    interface eth0
    virtual_router_id 51       # same as node A
    priority 90                # lower than node A

    advert_int 1

    authentication {
        auth_type PASS
        auth_pass somesecret   # same as node A
    }

    virtual_ipaddress {
        10.0.0.3/24            # same VIP
    }

    track_script {
        check_athena
    }

    notify_master "/etc/keepalived/notify.sh MASTER"
    notify_backup "/etc/keepalived/notify.sh BACKUP"
    notify_fault  "/etc/keepalived/notify.sh FAULT"
}
```

### optional notify script

useful for logging VIP transitions:

```bash
#!/bin/bash
# /etc/keepalived/notify.sh

STATE=$1
TIMESTAMP=$(date -Iseconds)

echo "$TIMESTAMP keepalived transitioning to $STATE" >> /var/log/keepalived-athena.log

case $STATE in
    MASTER)
        logger -t keepalived "athena VIP acquired — this node is now DNS master"
        ;;
    BACKUP)
        logger -t keepalived "athena VIP released — this node is now DNS backup"
        ;;
    FAULT)
        logger -t keepalived "athena VIP fault — health check failing"
        ;;
esac
```

```bash
sudo chmod +x /etc/keepalived/notify.sh
```

### start keepalived

```bash
sudo systemctl enable --now keepalived
```

verify the VIP is assigned on the primary:
```bash
ip addr show eth0 | grep 10.0.0.3
```

you should see `10.0.0.3/24` listed as a secondary address

## athena-dhcpd config

### DNS proxy — bind to all addresses

make sure the DNS proxy binds to `0.0.0.0` (the default) so it responds on the VIP:

```toml
[dns]
enabled = true
listen_udp = "0.0.0.0:53"
```

if you bind to a specific IP, it wont answer queries on the VIP when it floats over

### hand out the VIP as DNS server

tell DHCP clients to use the floating IP for DNS:

```toml
[defaults]
dns_servers = ["10.0.0.3"]    # the VIP, not a physical IP
domain_name = "home.lan"
```

or per-subnet:
```toml
[[subnet]]
network = "10.0.0.0/24"
dns_servers = ["10.0.0.3"]
```

### HA config

standard HA config on both nodes. the HA peer addresses use the real IPs, not the VIP:

```toml
# node A
[ha]
enabled = true
role = "primary"
peer_address = "10.0.0.2:8067"    # node B's real IP
listen_address = "0.0.0.0:8067"
```

```toml
# node B
[ha]
enabled = true
role = "secondary"
peer_address = "10.0.0.1:8067"    # node A's real IP
listen_address = "0.0.0.0:8067"
```

## how it all works together

normal operation:
1. node A is ACTIVE, node B is STANDBY
2. keepalived health check passes on node A → node A holds VIP `10.0.0.3`
3. clients send DNS queries to `10.0.0.3` → node A answers
4. leases and conflict table continuously synced to node B

failover:
1. node A crashes
2. keepalived health check fails on node A (3 consecutive failures = 6 seconds)
3. athena-dhcpd on node B detects peer down (`failover_timeout = 10s`)
4. node B transitions to ACTIVE
5. keepalived health check now passes on node B → node B acquires VIP
6. gratuitous ARP sent for `10.0.0.3` → switches update their MAC tables
7. DNS queries to `10.0.0.3` now reach node B
8. total DNS downtime: roughly 6-12 seconds

recovery:
1. node A comes back online
2. athena-dhcpd peers reconnect, bulk sync happens
3. node A goes back to ACTIVE, node B to STANDBY
4. keepalived on node A sees health check pass → node A reclaims VIP (higher priority)
5. gratuitous ARP announces VIP is back on node A

## alternative: floating IP without keepalived

if you dont want to run keepalived, you can manage the VIP yourself using athena-dhcpd's event hooks

### hook script approach

create a script hook that runs on HA state changes:

```bash
#!/bin/bash
# /etc/athena-dhcpd/hooks/floating-ip.sh

VIP="10.0.0.3/24"
INTERFACE="eth0"

case "$ATHENA_EVENT_TYPE" in
    ha.failover)
        if [ "$ATHENA_HA_NEW_STATE" = "ACTIVE" ]; then
            # we're now active — claim the VIP
            ip addr add $VIP dev $INTERFACE 2>/dev/null
            # send gratuitous ARP to update switch MAC tables
            arping -c 3 -U -I $INTERFACE ${VIP%/*}
            logger -t athena "claimed floating IP $VIP"
        else
            # we're no longer active — release the VIP
            ip addr del $VIP dev $INTERFACE 2>/dev/null
            logger -t athena "released floating IP $VIP"
        fi
        ;;
esac
```

```bash
sudo chmod +x /etc/athena-dhcpd/hooks/floating-ip.sh
```

configure the hook:

```toml
[[hooks.script]]
name = "floating-ip"
events = ["ha.failover"]
command = "/etc/athena-dhcpd/hooks/floating-ip.sh"
timeout = "5s"
```

this is simpler than keepalived but less robust — theres no independent health checking, no VRRP protocol, and no preemption. the VIP only moves when athena-dhcpd's own HA state machine fires. if the process is hung but not crashed, the VIP might not move

**the hook script needs root privileges** (or `CAP_NET_ADMIN`) to add/remove IP addresses. if athena-dhcpd runs as a non-root user, you'll need to either:
- give the script a sudo rule: `athena-dhcpd ALL=(root) NOPASSWD: /etc/athena-dhcpd/hooks/floating-ip.sh`
- use `setcap cap_net_admin+ep` on `arping` and `ip`
- wrap it in a small suid helper

keepalived is the better option for production

## firewall considerations

the VIP needs the same firewall rules as the physical IPs:

```bash
# allow DNS on the VIP
sudo ufw allow in on eth0 to 10.0.0.3 port 53 proto udp
sudo ufw allow in on eth0 to 10.0.0.3 port 53 proto tcp

# if using keepalived, allow VRRP between nodes
sudo ufw allow in on eth0 proto vrrp
# or more specifically:
sudo ufw allow from 10.0.0.1 to 224.0.0.18 proto vrrp
sudo ufw allow from 10.0.0.2 to 224.0.0.18 proto vrrp
```

keepalived uses VRRP (IP protocol 112) with multicast address `224.0.0.18`. if you block this, failover wont work

## testing failover

1. verify VIP is on node A:
   ```bash
   ip addr show eth0 | grep 10.0.0.3
   ```

2. query DNS through the VIP:
   ```bash
   dig @10.0.0.3 google.com
   ```

3. stop athena-dhcpd on node A:
   ```bash
   sudo systemctl stop athena-dhcpd
   ```

4. wait ~10 seconds, then check VIP moved to node B:
   ```bash
   # on node B
   ip addr show eth0 | grep 10.0.0.3
   ```

5. verify DNS still works through the VIP:
   ```bash
   dig @10.0.0.3 google.com
   ```

6. restart node A and verify VIP moves back:
   ```bash
   sudo systemctl start athena-dhcpd
   # wait for recovery + keepalived preemption
   ip addr show eth0 | grep 10.0.0.3
   ```

## troubleshooting

**VIP not appearing on either node**
- check keepalived is running: `systemctl status keepalived`
- check the health script works manually: `/etc/keepalived/check_athena.sh; echo $?`
- check VRRP traffic isnt blocked: `tcpdump -i eth0 vrrp`

**VIP on wrong node**
- check priorities are correct (primary should be higher)
- check health script passes on the correct node
- force failover: `sudo systemctl restart keepalived`

**DNS queries to VIP timing out**
- verify athena-dhcpd DNS proxy is enabled and listening on 0.0.0.0
- check: `ss -ulnp | grep :53`
- make sure the VIP is on the correct interface

**split brain — VIP on both nodes**
- usually means VRRP traffic is blocked between nodes
- check firewall rules for protocol 112 / multicast 224.0.0.18
- check `virtual_router_id` matches on both nodes
- check `auth_pass` matches on both nodes
