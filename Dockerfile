# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /athena-dhcpd ./cmd/athena-dhcpd

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S athena && adduser -S athena -G athena && \
    mkdir -p /etc/athena-dhcpd /var/lib/athena-dhcpd && \
    chown athena:athena /var/lib/athena-dhcpd

COPY --from=builder /athena-dhcpd /usr/local/bin/athena-dhcpd
COPY configs/example.toml /etc/athena-dhcpd/config.toml

# CAP_NET_RAW required for ARP/ICMP conflict detection + gratuitous ARP
# CAP_NET_BIND_SERVICE required for binding to port 67 (DHCP) and 53 (DNS)
# CAP_NET_ADMIN required for floating VIP management (ip addr add/del)
# Set via: docker run --cap-add=NET_RAW --cap-add=NET_BIND_SERVICE --cap-add=NET_ADMIN
# Or in docker-compose: cap_add: [NET_RAW, NET_BIND_SERVICE, NET_ADMIN]

USER athena

EXPOSE 67/udp
EXPOSE 8080/tcp

VOLUME ["/var/lib/athena-dhcpd", "/etc/athena-dhcpd"]

ENTRYPOINT ["/usr/local/bin/athena-dhcpd"]
CMD ["-config", "/etc/athena-dhcpd/config.toml"]
