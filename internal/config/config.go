// Package config handles TOML configuration parsing, validation, and hot-reload for athena-dhcpd.
package config

import (
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
	Enabled              bool   `toml:"enabled"`
	ProbeStrategy        string `toml:"probe_strategy"`
	ProbeTimeout         string `toml:"probe_timeout"`
	MaxProbesPerDiscover int    `toml:"max_probes_per_discover"`
	ParallelProbeCount   int    `toml:"parallel_probe_count"`
	ConflictHoldTime     string `toml:"conflict_hold_time"`
	MaxConflictCount     int    `toml:"max_conflict_count"`
	ProbeCacheTTL        string `toml:"probe_cache_ttl"`
	SendGratuitousARP    bool   `toml:"send_gratuitous_arp"`
	ICMPFallback         bool   `toml:"icmp_fallback"`
	ProbeLogLevel        string `toml:"probe_log_level"`
}

// HAConfig holds high availability settings.
type HAConfig struct {
	Enabled           bool        `toml:"enabled"`
	Role              string      `toml:"role"`
	PeerAddress       string      `toml:"peer_address"`
	ListenAddress     string      `toml:"listen_address"`
	HeartbeatInterval string      `toml:"heartbeat_interval"`
	FailoverTimeout   string      `toml:"failover_timeout"`
	SyncBatchSize     int         `toml:"sync_batch_size"`
	TLS               HATLSConfig `toml:"tls"`
}

// HATLSConfig holds TLS settings for HA peer communication.
type HATLSConfig struct {
	Enabled  bool   `toml:"enabled"`
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
	CAFile   string `toml:"ca_file"`
}

// HooksConfig holds event hook settings.
type HooksConfig struct {
	EventBufferSize   int           `toml:"event_buffer_size"`
	ScriptConcurrency int           `toml:"script_concurrency"`
	ScriptTimeout     string        `toml:"script_timeout"`
	Scripts           []ScriptHook  `toml:"script"`
	Webhooks          []WebhookHook `toml:"webhook"`
}

// ScriptHook defines a script hook.
type ScriptHook struct {
	Name    string   `toml:"name"`
	Events  []string `toml:"events"`
	Command string   `toml:"command"`
	Timeout string   `toml:"timeout"`
	Subnets []string `toml:"subnets"`
}

// WebhookHook defines a webhook hook.
type WebhookHook struct {
	Name         string            `toml:"name"`
	Events       []string          `toml:"events"`
	URL          string            `toml:"url"`
	Method       string            `toml:"method"`
	Headers      map[string]string `toml:"headers"`
	Timeout      string            `toml:"timeout"`
	Retries      int               `toml:"retries"`
	RetryBackoff string            `toml:"retry_backoff"`
	Secret       string            `toml:"secret"`
	Template     string            `toml:"template"`
}

// DDNSConfig holds dynamic DNS settings.
type DDNSConfig struct {
	Enabled         bool               `toml:"enabled"`
	AllowClientFQDN bool               `toml:"allow_client_fqdn"`
	FallbackToMAC   bool               `toml:"fallback_to_mac"`
	TTL             int                `toml:"ttl"`
	UpdateOnRenew   bool               `toml:"update_on_renew"`
	ConflictPolicy  string             `toml:"conflict_policy"`
	UseDHCID        bool               `toml:"use_dhcid"`
	Forward         DDNSZoneConfig     `toml:"forward"`
	Reverse         DDNSZoneConfig     `toml:"reverse"`
	ZoneOverrides   []DDNSZoneOverride `toml:"zone_override"`
}

// DDNSZoneConfig holds DNS zone configuration.
type DDNSZoneConfig struct {
	Zone          string `toml:"zone"`
	Method        string `toml:"method"`
	Server        string `toml:"server"`
	TSIGName      string `toml:"tsig_name"`
	TSIGAlgorithm string `toml:"tsig_algorithm"`
	TSIGSecret    string `toml:"tsig_secret"`
	APIKey        string `toml:"api_key"`
}

// DDNSZoneOverride holds per-subnet DDNS zone overrides.
type DDNSZoneOverride struct {
	Subnet        string `toml:"subnet"`
	ForwardZone   string `toml:"forward_zone"`
	ReverseZone   string `toml:"reverse_zone"`
	Method        string `toml:"method"`
	Server        string `toml:"server"`
	APIKey        string `toml:"api_key"`
	TSIGName      string `toml:"tsig_name"`
	TSIGAlgorithm string `toml:"tsig_algorithm"`
	TSIGSecret    string `toml:"tsig_secret"`
}

// SubnetConfig holds per-subnet configuration.
type SubnetConfig struct {
	Network      string              `toml:"network"`
	Routers      []string            `toml:"routers"`
	DNSServers   []string            `toml:"dns_servers"`
	DomainName   string              `toml:"domain_name"`
	LeaseTime    string              `toml:"lease_time"`
	RenewalTime  string              `toml:"renewal_time"`
	RebindTime   string              `toml:"rebind_time"`
	NTPServers   []string            `toml:"ntp_servers"`
	Pools        []PoolConfig        `toml:"pool"`
	Reservations []ReservationConfig `toml:"reservation"`
	Options      []OptionConfig      `toml:"option"`
}

// PoolConfig holds IP pool configuration.
type PoolConfig struct {
	RangeStart       string `toml:"range_start"`
	RangeEnd         string `toml:"range_end"`
	LeaseTime        string `toml:"lease_time"`
	MatchCircuitID   string `toml:"match_circuit_id"`
	MatchRemoteID    string `toml:"match_remote_id"`
	MatchVendorClass string `toml:"match_vendor_class"`
	MatchUserClass   string `toml:"match_user_class"`
}

// ReservationConfig holds static lease (reservation) configuration.
type ReservationConfig struct {
	MAC          string   `toml:"mac"`
	Identifier   string   `toml:"identifier"`
	IP           string   `toml:"ip"`
	Hostname     string   `toml:"hostname"`
	DNSServers   []string `toml:"dns_servers"`
	DDNSHostname string   `toml:"ddns_hostname"`
}

// OptionConfig holds custom DHCP option configuration.
type OptionConfig struct {
	Code  int         `toml:"code"`
	Type  string      `toml:"type"`
	Value interface{} `toml:"value"`
}

// DefaultsConfig holds global default option values.
type DefaultsConfig struct {
	LeaseTime   string   `toml:"lease_time"`
	RenewalTime string   `toml:"renewal_time"`
	RebindTime  string   `toml:"rebind_time"`
	DNSServers  []string `toml:"dns_servers"`
	DomainName  string   `toml:"domain_name"`
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
