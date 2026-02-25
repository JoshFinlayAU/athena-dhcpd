# athena-dhcpd Documentation

welcome to the docs. pick what you need

## guides

- **[Configuration Reference](configuration.md)** — every config field, every option, every default. the whole TOML explained
- **[API Reference](api.md)** — all REST endpoints, authentication, WebSocket streaming, request/response formats
- **[Deployment Guide](deployment.md)** — systemd, Docker, capabilities, firewalls, security hardening, backup strategy

## feature deep-dives

- **[Conflict Detection](conflict-detection.md)** — ARP/ICMP probing, conflict table, probe cache, DHCPDECLINE, gratuitous ARP. the whole thing
- **[Dynamic DNS](dynamic-dns.md)** — RFC 2136, PowerDNS API, Technitium API, FQDN construction, zone overrides, DHCID
- **[High Availability](high-availability.md)** — active-standby failover, lease sync, state machine, recovery, wire protocol
- **[Event Hooks](event-hooks.md)** — script hooks, webhooks, HMAC signing, Slack/Teams templates, environment variables
- **[Web UI](web-ui.md)** — pages, live updates, development workflow, building

## operations

- **[Monitoring](monitoring.md)** — every Prometheus metric, example queries, Grafana dashboard suggestions, alerting rules, structured logging

## quick links

- example config: [`configs/example.toml`](../configs/example.toml)
- systemd service: [`deploy/athena-dhcpd.service`](../deploy/athena-dhcpd.service)
- Dockerfile: [`Dockerfile`](../Dockerfile)

## architecture

- **[Architecture Overview](architecture.md)** — package layout, design principles, concurrency model, data flow
