// Package metrics defines all Prometheus metrics for athena-dhcpd.
// All metrics use the "athena_dhcpd_" prefix.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "athena_dhcpd"

// --- DHCP Packet Metrics ---

var (
	// PacketsReceived counts DHCP packets received by message type.
	PacketsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "packets_received_total",
		Help:      "Total DHCP packets received, by message type.",
	}, []string{"msg_type"})

	// PacketsSent counts DHCP packets sent by message type.
	PacketsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "packets_sent_total",
		Help:      "Total DHCP packets sent, by message type.",
	}, []string{"msg_type"})

	// PacketErrors counts packet processing errors.
	PacketErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "packet_errors_total",
		Help:      "Total packet processing errors, by type.",
	}, []string{"type"})

	// PacketProcessingDuration tracks DHCP packet handling latency.
	PacketProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "packet_processing_duration_seconds",
		Help:      "DHCP packet processing duration in seconds.",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
	}, []string{"msg_type"})
)

// --- Lease Metrics ---

var (
	// LeasesActive is a gauge of currently active leases.
	LeasesActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "leases_active",
		Help:      "Number of currently active leases.",
	})

	// LeasesOffered is a gauge of currently offered (pending) leases.
	LeasesOffered = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "leases_offered",
		Help:      "Number of currently offered (pending) leases.",
	})

	// LeaseOperations counts lease state transitions.
	LeaseOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "lease_operations_total",
		Help:      "Total lease operations, by type (offer, ack, renew, release, decline, expire).",
	}, []string{"operation"})
)

// --- Pool Metrics ---

var (
	// PoolSize is the total IPs in each pool.
	PoolSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "pool_size",
		Help:      "Total number of IPs in the pool.",
	}, []string{"subnet", "pool"})

	// PoolAllocated is the allocated IPs in each pool.
	PoolAllocated = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "pool_allocated",
		Help:      "Number of allocated IPs in the pool.",
	}, []string{"subnet", "pool"})

	// PoolUtilization is the utilization percentage of each pool.
	PoolUtilization = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "pool_utilization_percent",
		Help:      "Pool utilization as a percentage.",
	}, []string{"subnet", "pool"})

	// PoolExhausted counts pool exhaustion events.
	PoolExhausted = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "pool_exhausted_total",
		Help:      "Total times a pool was exhausted during allocation.",
	}, []string{"subnet"})
)

// --- Conflict Detection Metrics ---

var (
	// ConflictProbes counts conflict probes by method and result.
	ConflictProbes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "conflict_probes_total",
		Help:      "Total conflict probes performed.",
	}, []string{"method", "result"})

	// ConflictProbeDuration tracks probe latency by method.
	ConflictProbeDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "conflict_probe_duration_seconds",
		Help:      "Conflict probe duration in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0},
	}, []string{"method"})

	// ConflictsActive is a gauge of currently active conflicts.
	ConflictsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "conflicts_active",
		Help:      "Number of currently active IP conflicts.",
	}, []string{"subnet"})

	// ConflictsPermanent is a gauge of permanently flagged conflicts.
	ConflictsPermanent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "conflicts_permanent",
		Help:      "Number of permanently flagged IP conflicts.",
	}, []string{"subnet"})

	// ConflictsResolved counts resolved conflicts by method.
	ConflictsResolved = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "conflicts_resolved_total",
		Help:      "Total resolved conflicts.",
	}, []string{"method"})

	// ConflictDeclines counts DHCPDECLINE-triggered conflicts.
	ConflictDeclines = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "conflict_declines_total",
		Help:      "Total DHCPDECLINE-triggered conflicts.",
	}, []string{"subnet"})

	// ProbeCacheHits counts probe cache hits.
	ProbeCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "probe_cache_hits_total",
		Help:      "Total probe cache hits.",
	})

	// ProbeCacheMisses counts probe cache misses.
	ProbeCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "probe_cache_misses_total",
		Help:      "Total probe cache misses.",
	})
)

// --- Event Bus Metrics ---

var (
	// EventsPublished counts events published to the bus.
	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_published_total",
		Help:      "Total events published to the event bus.",
	}, []string{"event_type"})

	// EventBufferDrops counts events dropped due to full buffer.
	EventBufferDrops = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "event_buffer_drops_total",
		Help:      "Total events dropped due to full event bus buffer.",
	})

	// HookExecutions counts hook executions by type and result.
	HookExecutions = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "hook_executions_total",
		Help:      "Total hook executions.",
	}, []string{"hook_type", "result"})

	// HookDuration tracks hook execution latency.
	HookDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "hook_execution_duration_seconds",
		Help:      "Hook execution duration in seconds.",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0, 30.0},
	}, []string{"hook_type"})
)

// --- HA Metrics ---

var (
	// HAState reports the current HA state as a labeled gauge.
	HAState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "ha_state",
		Help:      "Current HA state (1 = current). Labels: state, role.",
	}, []string{"state", "role"})

	// HAHeartbeatsSent counts heartbeats sent to peer.
	HAHeartbeatsSent = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ha_heartbeats_sent_total",
		Help:      "Total heartbeats sent to HA peer.",
	})

	// HAHeartbeatsReceived counts heartbeats received from peer.
	HAHeartbeatsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ha_heartbeats_received_total",
		Help:      "Total heartbeats received from HA peer.",
	})

	// HASyncOperations counts lease sync operations by type.
	HASyncOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ha_sync_operations_total",
		Help:      "Total HA sync operations.",
	}, []string{"type"})

	// HASyncErrors counts HA sync errors.
	HASyncErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ha_sync_errors_total",
		Help:      "Total HA sync errors.",
	})
)

// --- API Metrics ---

var (
	// APIRequests counts HTTP API requests by method, path, and status.
	APIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "api_requests_total",
		Help:      "Total HTTP API requests.",
	}, []string{"method", "path", "status"})

	// APIRequestDuration tracks API request latency.
	APIRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "api_request_duration_seconds",
		Help:      "HTTP API request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})

	// SSEConnections is a gauge of active SSE (Server-Sent Events) connections.
	SSEConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "sse_connections_active",
		Help:      "Number of active SSE connections.",
	})
)

// --- DDNS Metrics ---

var (
	// DDNSUpdates counts DNS update operations by type and result.
	DDNSUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ddns_updates_total",
		Help:      "Total DDNS update operations.",
	}, []string{"type", "result"})

	// DDNSDuration tracks DNS update latency.
	DDNSDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "ddns_update_duration_seconds",
		Help:      "DDNS update duration in seconds.",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0},
	}, []string{"type"})
)

// --- DNS Proxy Metrics ---

var (
	// DNSQueriesTotal counts DNS queries by type and result status.
	DNSQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dns_queries_total",
		Help:      "Total DNS queries by query type and result status.",
	}, []string{"qtype", "status"})

	// DNSQueryDuration tracks DNS query processing latency.
	DNSQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "dns_query_duration_seconds",
		Help:      "DNS query processing duration in seconds.",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
	}, []string{"status"})

	// DNSCacheSize is the current number of entries in the DNS cache.
	DNSCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dns_cache_entries",
		Help:      "Current number of entries in the DNS response cache.",
	})

	// DNSCacheHits counts DNS cache hits.
	DNSCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dns_cache_hits_total",
		Help:      "Total DNS cache hits.",
	})

	// DNSCacheMisses counts DNS cache misses.
	DNSCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dns_cache_misses_total",
		Help:      "Total DNS cache misses.",
	})

	// DNSBlockedTotal counts blocked DNS queries by list name.
	DNSBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dns_blocked_total",
		Help:      "Total DNS queries blocked by filter lists.",
	}, []string{"list", "action"})

	// DNSZoneRecords is the current number of records in the local zone.
	DNSZoneRecords = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dns_zone_records",
		Help:      "Current number of records in the local DNS zone.",
	})

	// DNSUpstreamErrors counts failed upstream DNS forwards.
	DNSUpstreamErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dns_upstream_errors_total",
		Help:      "Total failed DNS upstream forward attempts.",
	})
)

// --- Server Info ---

var (
	// ServerInfo is a constant gauge with server metadata.
	ServerInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "server_info",
		Help:      "Server build and version info.",
	}, []string{"version"})

	// ServerUptime tracks server start time as a unix timestamp.
	ServerStartTime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "server_start_time_seconds",
		Help:      "Server start time as Unix timestamp.",
	})
)
