// Package config handles TOML configuration parsing, validation, and hot-reload for athena-dhcpd.
package config

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration for athena-dhcpd.
type Config struct {
	Server            ServerConfig            `toml:"server"`
	ConflictDetection ConflictDetectionConfig `toml:"conflict_detection"`
	HA                HAConfig                `toml:"ha"`
	Hooks             HooksConfig             `toml:"hooks"`
	DDNS              DDNSConfig              `toml:"ddns"`
	DNS               DNSProxyConfig          `toml:"dns"`
	Subnets           []SubnetConfig          `toml:"subnet"`
	Defaults          DefaultsConfig          `toml:"defaults"`
	API               APIConfig               `toml:"api"`
}

// ServerConfig holds core server settings.
type ServerConfig struct {
	Interface   string          `toml:"interface"`
	BindAddress string          `toml:"bind_address"`
	ServerID    string          `toml:"server_id"`
	LogLevel    string          `toml:"log_level"`
	LeaseDB     string          `toml:"lease_db"`
	PIDFile     string          `toml:"pid_file"`
	RateLimit   RateLimitConfig `toml:"rate_limit"`
}

// RateLimitConfig holds anti-starvation settings (RFC 5765).
type RateLimitConfig struct {
	Enabled               bool `toml:"enabled"`
	MaxDiscoversPerSecond int  `toml:"max_discovers_per_second"`
	MaxPerMACPerSecond    int  `toml:"max_per_mac_per_second"`
}

// ConflictDetectionConfig holds IP conflict detection settings.
type ConflictDetectionConfig struct {
	Enabled              bool   `toml:"enabled" json:"enabled"`
	ProbeStrategy        string `toml:"probe_strategy" json:"probe_strategy"`
	ProbeTimeout         string `toml:"probe_timeout" json:"probe_timeout"`
	MaxProbesPerDiscover int    `toml:"max_probes_per_discover" json:"max_probes_per_discover"`
	ParallelProbeCount   int    `toml:"parallel_probe_count" json:"parallel_probe_count"`
	ConflictHoldTime     string `toml:"conflict_hold_time" json:"conflict_hold_time"`
	MaxConflictCount     int    `toml:"max_conflict_count" json:"max_conflict_count"`
	ProbeCacheTTL        string `toml:"probe_cache_ttl" json:"probe_cache_ttl"`
	SendGratuitousARP    bool   `toml:"send_gratuitous_arp" json:"send_gratuitous_arp"`
	ICMPFallback         bool   `toml:"icmp_fallback" json:"icmp_fallback"`
	ProbeLogLevel        string `toml:"probe_log_level" json:"probe_log_level,omitempty"`
}

// HAConfig holds high availability settings.
type HAConfig struct {
	Enabled           bool        `toml:"enabled" json:"enabled"`
	Role              string      `toml:"role" json:"role"`
	PeerAddress       string      `toml:"peer_address" json:"peer_address"`
	ListenAddress     string      `toml:"listen_address" json:"listen_address"`
	HeartbeatInterval string      `toml:"heartbeat_interval" json:"heartbeat_interval"`
	FailoverTimeout   string      `toml:"failover_timeout" json:"failover_timeout"`
	SyncBatchSize     int         `toml:"sync_batch_size" json:"sync_batch_size"`
	TLS               HATLSConfig `toml:"tls" json:"tls"`
}

// HATLSConfig holds TLS settings for HA peer communication.
type HATLSConfig struct {
	Enabled  bool   `toml:"enabled" json:"enabled"`
	CertFile string `toml:"cert_file" json:"cert_file"`
	KeyFile  string `toml:"key_file" json:"key_file"`
	CAFile   string `toml:"ca_file" json:"ca_file"`
}

// HooksConfig holds event hook settings.
type HooksConfig struct {
	EventBufferSize   int           `toml:"event_buffer_size" json:"event_buffer_size"`
	ScriptConcurrency int           `toml:"script_concurrency" json:"script_concurrency"`
	ScriptTimeout     string        `toml:"script_timeout" json:"script_timeout"`
	Scripts           []ScriptHook  `toml:"script" json:"script,omitempty"`
	Webhooks          []WebhookHook `toml:"webhook" json:"webhook,omitempty"`
}

// ScriptHook defines a script hook.
type ScriptHook struct {
	Name    string   `toml:"name" json:"name"`
	Events  []string `toml:"events" json:"events"`
	Command string   `toml:"command" json:"command"`
	Timeout string   `toml:"timeout" json:"timeout"`
	Subnets []string `toml:"subnets" json:"subnets,omitempty"`
}

// WebhookHook defines a webhook hook.
type WebhookHook struct {
	Name         string            `toml:"name" json:"name"`
	Events       []string          `toml:"events" json:"events"`
	URL          string            `toml:"url" json:"url"`
	Method       string            `toml:"method" json:"method"`
	Headers      map[string]string `toml:"headers" json:"headers,omitempty"`
	Timeout      string            `toml:"timeout" json:"timeout"`
	Retries      int               `toml:"retries" json:"retries"`
	RetryBackoff string            `toml:"retry_backoff" json:"retry_backoff"`
	Secret       string            `toml:"secret" json:"secret,omitempty"`
	Template     string            `toml:"template" json:"template,omitempty"`
}

// DDNSConfig holds dynamic DNS settings.
type DDNSConfig struct {
	Enabled         bool               `toml:"enabled" json:"enabled"`
	AllowClientFQDN bool               `toml:"allow_client_fqdn" json:"allow_client_fqdn"`
	FallbackToMAC   bool               `toml:"fallback_to_mac" json:"fallback_to_mac"`
	TTL             int                `toml:"ttl" json:"ttl"`
	UpdateOnRenew   bool               `toml:"update_on_renew" json:"update_on_renew"`
	ConflictPolicy  string             `toml:"conflict_policy" json:"conflict_policy"`
	UseDHCID        bool               `toml:"use_dhcid" json:"use_dhcid"`
	Forward         DDNSZoneConfig     `toml:"forward" json:"forward"`
	Reverse         DDNSZoneConfig     `toml:"reverse" json:"reverse"`
	ZoneOverrides   []DDNSZoneOverride `toml:"zone_override" json:"zone_override,omitempty"`
}

// DDNSZoneConfig holds DNS zone configuration.
type DDNSZoneConfig struct {
	Zone          string `toml:"zone" json:"zone"`
	Method        string `toml:"method" json:"method"`
	Server        string `toml:"server" json:"server"`
	TSIGName      string `toml:"tsig_name" json:"tsig_name"`
	TSIGAlgorithm string `toml:"tsig_algorithm" json:"tsig_algorithm"`
	TSIGSecret    string `toml:"tsig_secret" json:"tsig_secret,omitempty"`
	APIKey        string `toml:"api_key" json:"api_key,omitempty"`
}

// DDNSZoneOverride holds per-subnet DDNS zone overrides.
type DDNSZoneOverride struct {
	Subnet        string `toml:"subnet" json:"subnet"`
	ForwardZone   string `toml:"forward_zone" json:"forward_zone"`
	ReverseZone   string `toml:"reverse_zone" json:"reverse_zone"`
	Method        string `toml:"method" json:"method"`
	Server        string `toml:"server" json:"server"`
	APIKey        string `toml:"api_key" json:"api_key,omitempty"`
	TSIGName      string `toml:"tsig_name" json:"tsig_name"`
	TSIGAlgorithm string `toml:"tsig_algorithm" json:"tsig_algorithm"`
	TSIGSecret    string `toml:"tsig_secret" json:"tsig_secret,omitempty"`
}

// DNSProxyConfig holds built-in DNS proxy settings.
type DNSProxyConfig struct {
	Enabled          bool              `toml:"enabled" json:"enabled"`
	ListenUDP        string            `toml:"listen_udp" json:"listen_udp"`
	ListenDoH        string            `toml:"listen_doh" json:"listen_doh,omitempty"`
	DoHTLS           DoHTLSConfig      `toml:"doh_tls" json:"doh_tls,omitempty"`
	Domain           string            `toml:"domain" json:"domain"`
	TTL              int               `toml:"ttl" json:"ttl"`
	RegisterLeases   bool              `toml:"register_leases" json:"register_leases"`
	ForwardLeasesPTR bool              `toml:"register_leases_ptr" json:"register_leases_ptr"`
	Forwarders       []string          `toml:"forwarders" json:"forwarders"`
	UseRootServers   bool              `toml:"use_root_servers" json:"use_root_servers"`
	CacheSize        int               `toml:"cache_size" json:"cache_size"`
	CacheTTL         string            `toml:"cache_ttl" json:"cache_ttl"`
	ZoneOverrides    []DNSZoneOverride `toml:"zone_override" json:"zone_override,omitempty"`
	StaticRecords    []DNSStaticRecord `toml:"record" json:"record,omitempty"`
	Lists            []DNSListConfig   `toml:"list" json:"list,omitempty"`
}

// DoHTLSConfig holds TLS settings for DNS-over-HTTPS.
type DoHTLSConfig struct {
	CertFile string `toml:"cert_file" json:"cert_file,omitempty"`
	KeyFile  string `toml:"key_file" json:"key_file,omitempty"`
}

// DNSListConfig holds a dynamic DNS filter list (blocklist or allowlist).
type DNSListConfig struct {
	Name            string `toml:"name" json:"name"`
	URL             string `toml:"url" json:"url"`
	Type            string `toml:"type" json:"type"`     // "block" or "allow"
	Format          string `toml:"format" json:"format"` // "hosts", "domains", "adblock"
	Action          string `toml:"action" json:"action"` // "nxdomain", "zero", "refuse"
	Enabled         bool   `toml:"enabled" json:"enabled"`
	RefreshInterval string `toml:"refresh_interval" json:"refresh_interval"` // e.g. "24h", "6h"
}

// DNSZoneOverride routes queries for a specific domain to a specific nameserver.
type DNSZoneOverride struct {
	Zone       string `toml:"zone" json:"zone"`
	Nameserver string `toml:"nameserver" json:"nameserver"`
	DoH        bool   `toml:"doh" json:"doh"`
	DoHURL     string `toml:"doh_url" json:"doh_url,omitempty"`
}

// DNSStaticRecord defines a static DNS record.
type DNSStaticRecord struct {
	Name  string `toml:"name" json:"name"`
	Type  string `toml:"type" json:"type"`
	Value string `toml:"value" json:"value"`
	TTL   int    `toml:"ttl" json:"ttl"`
}

// SubnetConfig holds per-subnet configuration.
type SubnetConfig struct {
	Network      string              `toml:"network" json:"network"`
	Interface    string              `toml:"interface" json:"interface,omitempty"`
	Routers      []string            `toml:"routers" json:"routers,omitempty"`
	DNSServers   []string            `toml:"dns_servers" json:"dns_servers,omitempty"`
	DomainName   string              `toml:"domain_name" json:"domain_name,omitempty"`
	LeaseTime    string              `toml:"lease_time" json:"lease_time,omitempty"`
	RenewalTime  string              `toml:"renewal_time" json:"renewal_time,omitempty"`
	RebindTime   string              `toml:"rebind_time" json:"rebind_time,omitempty"`
	NTPServers   []string            `toml:"ntp_servers" json:"ntp_servers,omitempty"`
	Pools        []PoolConfig        `toml:"pool" json:"pool,omitempty"`
	Reservations []ReservationConfig `toml:"reservation" json:"reservation,omitempty"`
	Options      []OptionConfig      `toml:"option" json:"option,omitempty"`
}

// PoolConfig holds IP pool configuration.
type PoolConfig struct {
	RangeStart       string `toml:"range_start" json:"range_start"`
	RangeEnd         string `toml:"range_end" json:"range_end"`
	LeaseTime        string `toml:"lease_time" json:"lease_time,omitempty"`
	MatchCircuitID   string `toml:"match_circuit_id" json:"match_circuit_id,omitempty"`
	MatchRemoteID    string `toml:"match_remote_id" json:"match_remote_id,omitempty"`
	MatchVendorClass string `toml:"match_vendor_class" json:"match_vendor_class,omitempty"`
	MatchUserClass   string `toml:"match_user_class" json:"match_user_class,omitempty"`
}

// ReservationConfig holds static lease (reservation) configuration.
type ReservationConfig struct {
	MAC          string   `toml:"mac" json:"mac"`
	Identifier   string   `toml:"identifier" json:"identifier,omitempty"`
	IP           string   `toml:"ip" json:"ip"`
	Hostname     string   `toml:"hostname" json:"hostname,omitempty"`
	DNSServers   []string `toml:"dns_servers" json:"dns_servers,omitempty"`
	DDNSHostname string   `toml:"ddns_hostname" json:"ddns_hostname,omitempty"`
}

// OptionConfig holds custom DHCP option configuration.
type OptionConfig struct {
	Code  int         `toml:"code" json:"code"`
	Type  string      `toml:"type" json:"type"`
	Value interface{} `toml:"value" json:"value"`
}

// DefaultsConfig holds global default option values.
type DefaultsConfig struct {
	LeaseTime   string   `toml:"lease_time" json:"lease_time"`
	RenewalTime string   `toml:"renewal_time" json:"renewal_time"`
	RebindTime  string   `toml:"rebind_time" json:"rebind_time"`
	DNSServers  []string `toml:"dns_servers" json:"dns_servers"`
	DomainName  string   `toml:"domain_name" json:"domain_name"`
}

// APIConfig holds HTTP API and web UI settings.
type APIConfig struct {
	Enabled bool          `toml:"enabled"`
	Listen  string        `toml:"listen"`
	WebUI   bool          `toml:"web_ui"`
	Auth    APIAuthConfig `toml:"auth"`
	TLS     APITLSConfig  `toml:"tls"`
	Session SessionConfig `toml:"session"`
}

// APIAuthConfig holds auth settings.
type APIAuthConfig struct {
	AuthToken string       `toml:"auth_token"`
	Users     []UserConfig `toml:"users"`
}

// UserConfig holds a web UI user.
type UserConfig struct {
	Username     string `toml:"username"`
	PasswordHash string `toml:"password_hash"`
	Role         string `toml:"role"`
}

// APITLSConfig holds API TLS settings.
type APITLSConfig struct {
	Enabled  bool   `toml:"enabled"`
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
}

// SessionConfig holds session settings.
type SessionConfig struct {
	CookieName string `toml:"cookie_name"`
	Expiry     string `toml:"expiry"`
	Secure     bool   `toml:"secure"`
}

// Load reads and parses a TOML config file, applies defaults, and validates.
// This loads ALL sections (v1 compat). For v2, use LoadBootstrap + dbconfig.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// LoadBootstrap reads only the bootstrap sections (server + api) from TOML.
// All other config (subnets, defaults, hooks, etc.) comes from the database.
func LoadBootstrap(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	applyBootstrapDefaults(cfg)

	if err := validateBootstrap(cfg); err != nil {
		return nil, fmt.Errorf("validating bootstrap config: %w", err)
	}

	return cfg, nil
}

// HasDynamicConfig returns true if the TOML file contains any dynamic config
// sections (subnets, defaults, hooks, etc.) — used for v1 auto-migration.
func HasDynamicConfig(cfg *Config) bool {
	return len(cfg.Subnets) > 0 ||
		cfg.Defaults.LeaseTime != "" ||
		cfg.ConflictDetection.Enabled ||
		cfg.HA.Enabled ||
		cfg.DDNS.Enabled ||
		cfg.DNS.Enabled ||
		len(cfg.Hooks.Scripts) > 0 ||
		len(cfg.Hooks.Webhooks) > 0
}

// applyBootstrapDefaults fills defaults for server + api sections only.
func applyBootstrapDefaults(cfg *Config) {
	if cfg.Server.Interface == "" {
		cfg.Server.Interface = DefaultInterface
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = DefaultLogLevel
	}
	if cfg.Server.LeaseDB == "" {
		cfg.Server.LeaseDB = DefaultLeaseDB
	}
	if cfg.Server.PIDFile == "" {
		cfg.Server.PIDFile = DefaultPIDFile
	}
	if cfg.API.Listen == "" {
		cfg.API.Listen = DefaultAPIListen
	}
	if cfg.API.Session.CookieName == "" {
		cfg.API.Session.CookieName = DefaultSessionCookieName
	}
	if cfg.API.Session.Expiry == "" {
		cfg.API.Session.Expiry = DefaultSessionExpiry.String()
	}
}

// validateBootstrap checks only the bootstrap config sections.
func validateBootstrap(cfg *Config) error {
	if cfg.Server.ServerID != "" {
		if ip := net.ParseIP(cfg.Server.ServerID); ip == nil {
			return fmt.Errorf("server.server_id %q is not a valid IP address", cfg.Server.ServerID)
		}
	}
	return nil
}

// ApplyDynamicDefaults fills in default values for dynamic config sections
// (everything except server + api). Called after building config from DB.
func ApplyDynamicDefaults(cfg *Config) {
	// Rate limit defaults
	if cfg.Server.RateLimit.MaxDiscoversPerSecond == 0 {
		cfg.Server.RateLimit.MaxDiscoversPerSecond = DefaultRateLimitDiscovers
	}
	if cfg.Server.RateLimit.MaxPerMACPerSecond == 0 {
		cfg.Server.RateLimit.MaxPerMACPerSecond = DefaultRateLimitPerMAC
	}

	// Conflict detection defaults
	if cfg.ConflictDetection.ProbeStrategy == "" {
		cfg.ConflictDetection.ProbeStrategy = DefaultProbeStrategy
	}
	if cfg.ConflictDetection.ProbeTimeout == "" {
		cfg.ConflictDetection.ProbeTimeout = DefaultProbeTimeout.String()
	}
	if cfg.ConflictDetection.MaxProbesPerDiscover == 0 {
		cfg.ConflictDetection.MaxProbesPerDiscover = DefaultMaxProbesPerDiscover
	}
	if cfg.ConflictDetection.ParallelProbeCount == 0 {
		cfg.ConflictDetection.ParallelProbeCount = DefaultParallelProbeCount
	}
	if cfg.ConflictDetection.ConflictHoldTime == "" {
		cfg.ConflictDetection.ConflictHoldTime = DefaultConflictHoldTime.String()
	}
	if cfg.ConflictDetection.MaxConflictCount == 0 {
		cfg.ConflictDetection.MaxConflictCount = DefaultMaxConflictCount
	}
	if cfg.ConflictDetection.ProbeCacheTTL == "" {
		cfg.ConflictDetection.ProbeCacheTTL = DefaultProbeCacheTTL.String()
	}
	if cfg.ConflictDetection.ProbeLogLevel == "" {
		cfg.ConflictDetection.ProbeLogLevel = DefaultProbeLogLevel
	}

	// Hooks defaults
	if cfg.Hooks.EventBufferSize == 0 {
		cfg.Hooks.EventBufferSize = DefaultEventBufferSize
	}
	if cfg.Hooks.ScriptConcurrency == 0 {
		cfg.Hooks.ScriptConcurrency = DefaultScriptConcurrency
	}
	if cfg.Hooks.ScriptTimeout == "" {
		cfg.Hooks.ScriptTimeout = DefaultScriptTimeout.String()
	}

	// HA defaults
	if cfg.HA.HeartbeatInterval == "" {
		cfg.HA.HeartbeatInterval = DefaultHAHeartbeatInterval.String()
	}
	if cfg.HA.FailoverTimeout == "" {
		cfg.HA.FailoverTimeout = DefaultHAFailoverTimeout.String()
	}
	if cfg.HA.SyncBatchSize == 0 {
		cfg.HA.SyncBatchSize = DefaultHASyncBatchSize
	}

	// Global defaults
	if cfg.Defaults.LeaseTime == "" {
		cfg.Defaults.LeaseTime = DefaultLeaseTime.String()
	}
	if cfg.Defaults.RenewalTime == "" {
		cfg.Defaults.RenewalTime = DefaultRenewalTime.String()
	}
	if cfg.Defaults.RebindTime == "" {
		cfg.Defaults.RebindTime = DefaultRebindTime.String()
	}

	// DNS proxy defaults
	if cfg.DNS.ListenUDP == "" {
		cfg.DNS.ListenUDP = DefaultDNSListenUDP
	}
	if cfg.DNS.TTL == 0 {
		cfg.DNS.TTL = DefaultDNSTTL
	}
	if cfg.DNS.CacheSize == 0 {
		cfg.DNS.CacheSize = DefaultDNSCacheSize
	}
	if cfg.DNS.CacheTTL == "" {
		cfg.DNS.CacheTTL = DefaultDNSCacheTTL.String()
	}

	// DDNS defaults
	if cfg.DDNS.TTL == 0 {
		cfg.DDNS.TTL = DefaultDDNSTTL
	}
	if cfg.DDNS.ConflictPolicy == "" {
		cfg.DDNS.ConflictPolicy = DefaultDDNSConflictPolicy
	}

	// Webhook defaults
	for i := range cfg.Hooks.Webhooks {
		if cfg.Hooks.Webhooks[i].Method == "" {
			cfg.Hooks.Webhooks[i].Method = "POST"
		}
		if cfg.Hooks.Webhooks[i].Retries == 0 {
			cfg.Hooks.Webhooks[i].Retries = DefaultWebhookRetries
		}
		if cfg.Hooks.Webhooks[i].RetryBackoff == "" {
			cfg.Hooks.Webhooks[i].RetryBackoff = DefaultWebhookRetryBackoff.String()
		}
	}
}

// applyDefaults fills in default values for unset fields.
func applyDefaults(cfg *Config) {
	if cfg.Server.Interface == "" {
		cfg.Server.Interface = DefaultInterface
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = DefaultLogLevel
	}
	if cfg.Server.LeaseDB == "" {
		cfg.Server.LeaseDB = DefaultLeaseDB
	}
	if cfg.Server.PIDFile == "" {
		cfg.Server.PIDFile = DefaultPIDFile
	}
	if cfg.Server.RateLimit.MaxDiscoversPerSecond == 0 {
		cfg.Server.RateLimit.MaxDiscoversPerSecond = DefaultRateLimitDiscovers
	}
	if cfg.Server.RateLimit.MaxPerMACPerSecond == 0 {
		cfg.Server.RateLimit.MaxPerMACPerSecond = DefaultRateLimitPerMAC
	}

	// Conflict detection defaults
	if cfg.ConflictDetection.ProbeStrategy == "" {
		cfg.ConflictDetection.ProbeStrategy = DefaultProbeStrategy
	}
	if cfg.ConflictDetection.ProbeTimeout == "" {
		cfg.ConflictDetection.ProbeTimeout = DefaultProbeTimeout.String()
	}
	if cfg.ConflictDetection.MaxProbesPerDiscover == 0 {
		cfg.ConflictDetection.MaxProbesPerDiscover = DefaultMaxProbesPerDiscover
	}
	if cfg.ConflictDetection.ParallelProbeCount == 0 {
		cfg.ConflictDetection.ParallelProbeCount = DefaultParallelProbeCount
	}
	if cfg.ConflictDetection.ConflictHoldTime == "" {
		cfg.ConflictDetection.ConflictHoldTime = DefaultConflictHoldTime.String()
	}
	if cfg.ConflictDetection.MaxConflictCount == 0 {
		cfg.ConflictDetection.MaxConflictCount = DefaultMaxConflictCount
	}
	if cfg.ConflictDetection.ProbeCacheTTL == "" {
		cfg.ConflictDetection.ProbeCacheTTL = DefaultProbeCacheTTL.String()
	}
	if cfg.ConflictDetection.ProbeLogLevel == "" {
		cfg.ConflictDetection.ProbeLogLevel = DefaultProbeLogLevel
	}

	// Hooks defaults
	if cfg.Hooks.EventBufferSize == 0 {
		cfg.Hooks.EventBufferSize = DefaultEventBufferSize
	}
	if cfg.Hooks.ScriptConcurrency == 0 {
		cfg.Hooks.ScriptConcurrency = DefaultScriptConcurrency
	}
	if cfg.Hooks.ScriptTimeout == "" {
		cfg.Hooks.ScriptTimeout = DefaultScriptTimeout.String()
	}

	// HA defaults
	if cfg.HA.HeartbeatInterval == "" {
		cfg.HA.HeartbeatInterval = DefaultHAHeartbeatInterval.String()
	}
	if cfg.HA.FailoverTimeout == "" {
		cfg.HA.FailoverTimeout = DefaultHAFailoverTimeout.String()
	}
	if cfg.HA.SyncBatchSize == 0 {
		cfg.HA.SyncBatchSize = DefaultHASyncBatchSize
	}

	// Global defaults
	if cfg.Defaults.LeaseTime == "" {
		cfg.Defaults.LeaseTime = DefaultLeaseTime.String()
	}
	if cfg.Defaults.RenewalTime == "" {
		cfg.Defaults.RenewalTime = DefaultRenewalTime.String()
	}
	if cfg.Defaults.RebindTime == "" {
		cfg.Defaults.RebindTime = DefaultRebindTime.String()
	}

	// API defaults
	if cfg.API.Listen == "" {
		cfg.API.Listen = DefaultAPIListen
	}
	if cfg.API.Session.CookieName == "" {
		cfg.API.Session.CookieName = DefaultSessionCookieName
	}
	if cfg.API.Session.Expiry == "" {
		cfg.API.Session.Expiry = DefaultSessionExpiry.String()
	}

	// DNS proxy defaults
	if cfg.DNS.ListenUDP == "" {
		cfg.DNS.ListenUDP = DefaultDNSListenUDP
	}
	if cfg.DNS.TTL == 0 {
		cfg.DNS.TTL = DefaultDNSTTL
	}
	if cfg.DNS.CacheSize == 0 {
		cfg.DNS.CacheSize = DefaultDNSCacheSize
	}
	if cfg.DNS.CacheTTL == "" {
		cfg.DNS.CacheTTL = DefaultDNSCacheTTL.String()
	}

	// DDNS defaults
	if cfg.DDNS.TTL == 0 {
		cfg.DDNS.TTL = DefaultDDNSTTL
	}
	if cfg.DDNS.ConflictPolicy == "" {
		cfg.DDNS.ConflictPolicy = DefaultDDNSConflictPolicy
	}

	// Webhook defaults
	for i := range cfg.Hooks.Webhooks {
		if cfg.Hooks.Webhooks[i].Method == "" {
			cfg.Hooks.Webhooks[i].Method = "POST"
		}
		if cfg.Hooks.Webhooks[i].Retries == 0 {
			cfg.Hooks.Webhooks[i].Retries = DefaultWebhookRetries
		}
		if cfg.Hooks.Webhooks[i].RetryBackoff == "" {
			cfg.Hooks.Webhooks[i].RetryBackoff = DefaultWebhookRetryBackoff.String()
		}
	}
}

// validate checks the configuration for errors.
func validate(cfg *Config) error {
	// Server ID must be a valid IP
	if cfg.Server.ServerID != "" {
		if ip := net.ParseIP(cfg.Server.ServerID); ip == nil {
			return fmt.Errorf("server.server_id %q is not a valid IP address", cfg.Server.ServerID)
		}
	}

	// Validate conflict detection
	if cfg.ConflictDetection.Enabled {
		if cfg.ConflictDetection.ProbeStrategy != "sequential" && cfg.ConflictDetection.ProbeStrategy != "parallel" {
			return fmt.Errorf("conflict_detection.probe_strategy must be \"sequential\" or \"parallel\", got %q", cfg.ConflictDetection.ProbeStrategy)
		}
		if _, err := time.ParseDuration(cfg.ConflictDetection.ProbeTimeout); err != nil {
			return fmt.Errorf("conflict_detection.probe_timeout: %w", err)
		}
		if _, err := time.ParseDuration(cfg.ConflictDetection.ConflictHoldTime); err != nil {
			return fmt.Errorf("conflict_detection.conflict_hold_time: %w", err)
		}
		if _, err := time.ParseDuration(cfg.ConflictDetection.ProbeCacheTTL); err != nil {
			return fmt.Errorf("conflict_detection.probe_cache_ttl: %w", err)
		}
	}

	// Validate subnets
	for i, sub := range cfg.Subnets {
		if sub.Network == "" {
			return fmt.Errorf("subnet[%d]: network is required", i)
		}
		_, network, err := net.ParseCIDR(sub.Network)
		if err != nil {
			return fmt.Errorf("subnet[%d]: invalid network %q: %w", i, sub.Network, err)
		}

		// Validate pools
		for j, pool := range sub.Pools {
			start := net.ParseIP(pool.RangeStart)
			if start == nil {
				return fmt.Errorf("subnet[%d].pool[%d]: invalid range_start %q", i, j, pool.RangeStart)
			}
			end := net.ParseIP(pool.RangeEnd)
			if end == nil {
				return fmt.Errorf("subnet[%d].pool[%d]: invalid range_end %q", i, j, pool.RangeEnd)
			}
			if !network.Contains(start) {
				return fmt.Errorf("subnet[%d].pool[%d]: range_start %s is not in network %s", i, j, start, network)
			}
			if !network.Contains(end) {
				return fmt.Errorf("subnet[%d].pool[%d]: range_end %s is not in network %s", i, j, end, network)
			}
		}

		// Validate pool range ordering (end >= start)
		for j, pool := range sub.Pools {
			start := net.ParseIP(pool.RangeStart).To4()
			end := net.ParseIP(pool.RangeEnd).To4()
			if start != nil && end != nil {
				startU := binary.BigEndian.Uint32(start)
				endU := binary.BigEndian.Uint32(end)
				if endU < startU {
					return fmt.Errorf("subnet[%d].pool[%d]: range_end %s is before range_start %s", i, j, pool.RangeEnd, pool.RangeStart)
				}
			}
		}

		// Validate no overlapping pools within the subnet
		for j := 0; j < len(sub.Pools); j++ {
			for k := j + 1; k < len(sub.Pools); k++ {
				if poolsOverlap(sub.Pools[j], sub.Pools[k]) {
					return fmt.Errorf("subnet[%d]: pool[%d] (%s-%s) overlaps with pool[%d] (%s-%s)",
						i, j, sub.Pools[j].RangeStart, sub.Pools[j].RangeEnd,
						k, sub.Pools[k].RangeStart, sub.Pools[k].RangeEnd)
				}
			}
		}

		// Validate reservations
		for j, res := range sub.Reservations {
			if res.MAC == "" && res.Identifier == "" {
				return fmt.Errorf("subnet[%d].reservation[%d]: mac or identifier is required", i, j)
			}
			if res.IP == "" {
				return fmt.Errorf("subnet[%d].reservation[%d]: ip is required", i, j)
			}
			ip := net.ParseIP(res.IP)
			if ip == nil {
				return fmt.Errorf("subnet[%d].reservation[%d]: invalid ip %q", i, j, res.IP)
			}
			if !network.Contains(ip) {
				return fmt.Errorf("subnet[%d].reservation[%d]: ip %s is not in network %s", i, j, ip, network)
			}
		}

		// Validate duration fields
		if sub.LeaseTime != "" {
			if _, err := time.ParseDuration(sub.LeaseTime); err != nil {
				return fmt.Errorf("subnet[%d].lease_time: %w", i, err)
			}
		}
	}

	// Validate no overlapping subnets
	for i := 0; i < len(cfg.Subnets); i++ {
		for j := i + 1; j < len(cfg.Subnets); j++ {
			if subnetsOverlap(cfg.Subnets[i].Network, cfg.Subnets[j].Network) {
				return fmt.Errorf("subnet[%d] (%s) overlaps with subnet[%d] (%s)",
					i, cfg.Subnets[i].Network, j, cfg.Subnets[j].Network)
			}
		}
	}

	// Validate HA config
	if cfg.HA.Enabled {
		if cfg.HA.Role != "primary" && cfg.HA.Role != "secondary" {
			return fmt.Errorf("ha.role must be \"primary\" or \"secondary\", got %q", cfg.HA.Role)
		}
		if cfg.HA.PeerAddress == "" {
			return fmt.Errorf("ha.peer_address is required when HA is enabled")
		}
		if cfg.HA.ListenAddress == "" {
			return fmt.Errorf("ha.listen_address is required when HA is enabled")
		}
	}

	// Validate DDNS
	if cfg.DDNS.Enabled {
		if cfg.DDNS.Forward.Zone == "" {
			return fmt.Errorf("ddns.forward.zone is required when DDNS is enabled")
		}
		method := cfg.DDNS.Forward.Method
		if method != "rfc2136" && method != "powerdns_api" && method != "technitium_api" {
			return fmt.Errorf("ddns.forward.method must be rfc2136, powerdns_api, or technitium_api, got %q", method)
		}
	}

	return nil
}

// poolsOverlap returns true if two pool ranges overlap.
func poolsOverlap(a, b PoolConfig) bool {
	aStart := net.ParseIP(a.RangeStart).To4()
	aEnd := net.ParseIP(a.RangeEnd).To4()
	bStart := net.ParseIP(b.RangeStart).To4()
	bEnd := net.ParseIP(b.RangeEnd).To4()
	if aStart == nil || aEnd == nil || bStart == nil || bEnd == nil {
		return false
	}
	aS := binary.BigEndian.Uint32(aStart)
	aE := binary.BigEndian.Uint32(aEnd)
	bS := binary.BigEndian.Uint32(bStart)
	bE := binary.BigEndian.Uint32(bEnd)
	return aS <= bE && bS <= aE
}

// subnetsOverlap returns true if two subnet CIDRs overlap.
func subnetsOverlap(cidrA, cidrB string) bool {
	_, netA, errA := net.ParseCIDR(cidrA)
	_, netB, errB := net.ParseCIDR(cidrB)
	if errA != nil || errB != nil {
		return false
	}
	return netA.Contains(netB.IP) || netB.Contains(netA.IP)
}

// ParseDuration is a helper for parsing Go-style duration strings.
func ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// GetLeaseTime returns the effective lease time for a subnet, falling back to defaults.
func (cfg *Config) GetLeaseTime(subnetIdx int) time.Duration {
	if subnetIdx >= 0 && subnetIdx < len(cfg.Subnets) {
		if cfg.Subnets[subnetIdx].LeaseTime != "" {
			d, err := time.ParseDuration(cfg.Subnets[subnetIdx].LeaseTime)
			if err == nil {
				return d
			}
		}
	}
	d, err := time.ParseDuration(cfg.Defaults.LeaseTime)
	if err != nil {
		return DefaultLeaseTime
	}
	return d
}

// GetRenewalTime returns the effective renewal time (T1).
func (cfg *Config) GetRenewalTime(subnetIdx int) time.Duration {
	if subnetIdx >= 0 && subnetIdx < len(cfg.Subnets) {
		if cfg.Subnets[subnetIdx].RenewalTime != "" {
			d, err := time.ParseDuration(cfg.Subnets[subnetIdx].RenewalTime)
			if err == nil {
				return d
			}
		}
	}
	d, err := time.ParseDuration(cfg.Defaults.RenewalTime)
	if err != nil {
		return DefaultRenewalTime
	}
	return d
}

// GetRebindTime returns the effective rebind time (T2).
func (cfg *Config) GetRebindTime(subnetIdx int) time.Duration {
	if subnetIdx >= 0 && subnetIdx < len(cfg.Subnets) {
		if cfg.Subnets[subnetIdx].RebindTime != "" {
			d, err := time.ParseDuration(cfg.Subnets[subnetIdx].RebindTime)
			if err == nil {
				return d
			}
		}
	}
	d, err := time.ParseDuration(cfg.Defaults.RebindTime)
	if err != nil {
		return DefaultRebindTime
	}
	return d
}

// ServerIP returns the parsed server identifier IP.
func (cfg *Config) ServerIP() net.IP {
	if cfg.Server.ServerID == "" {
		return nil
	}
	return net.ParseIP(cfg.Server.ServerID)
}
