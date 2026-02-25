package config

import "time"

// Default configuration values.
const (
	DefaultInterface            = "eth0"
	DefaultLogLevel             = "info"
	DefaultLeaseDB              = "/var/lib/athena-dhcpd/leases.db"
	DefaultPIDFile              = "/run/athena-dhcpd.pid"
	DefaultLeaseTime            = 12 * time.Hour
	DefaultRenewalTime          = 6 * time.Hour
	DefaultRebindTime           = 10*time.Hour + 30*time.Minute
	DefaultEventBufferSize      = 10000
	DefaultScriptConcurrency    = 4
	DefaultScriptTimeout        = 10 * time.Second
	DefaultProbeTimeout         = 500 * time.Millisecond
	DefaultMaxProbesPerDiscover = 3
	DefaultParallelProbeCount   = 3
	DefaultConflictHoldTime     = 1 * time.Hour
	DefaultMaxConflictCount     = 3
	DefaultProbeCacheTTL        = 10 * time.Second
	DefaultProbeStrategy        = "sequential"
	DefaultProbeLogLevel        = "debug"
	DefaultHAHeartbeatInterval  = 1 * time.Second
	DefaultHAFailoverTimeout    = 10 * time.Second
	DefaultHASyncBatchSize      = 100
	DefaultAPIListen            = "0.0.0.0:8067"
	DefaultSessionExpiry        = 24 * time.Hour
	DefaultSessionCookieName    = "athena_session"
	DefaultWebhookRetries       = 3
	DefaultWebhookRetryBackoff  = 2 * time.Second
	DefaultDDNSTTL              = 300
	DefaultDDNSConflictPolicy   = "overwrite"
	DefaultRateLimitDiscovers   = 100
	DefaultRateLimitPerMAC      = 5
	DefaultDNSListenUDP         = "0.0.0.0:53"
	DefaultDNSListenDoH         = ""
	DefaultDNSTTL               = 60
	DefaultDNSCacheSize         = 10000
	DefaultDNSCacheTTL          = 5 * time.Minute
)
