// Package radius provides RADIUS authentication/authorization for DHCP clients.
// It supports per-subnet RADIUS server configuration and provides a test function
// to validate connectivity and credentials.
package radius

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"layeh.com/radius"
	"layeh.com/radius/rfc2865"
	"layeh.com/radius/rfc2869"
)

// ServerConfig holds RADIUS server settings.
type ServerConfig struct {
	Address string `json:"address" toml:"address"` // host:port
	Secret  string `json:"secret" toml:"secret"`   // shared secret
	Timeout string `json:"timeout" toml:"timeout"` // e.g. "5s"
	Retries int    `json:"retries" toml:"retries"` // number of retries
}

// SubnetConfig holds per-subnet RADIUS settings.
type SubnetConfig struct {
	Enabled        bool         `json:"enabled" toml:"enabled"`
	Server         ServerConfig `json:"server" toml:"server"`
	NASIdentifier  string       `json:"nas_identifier" toml:"nas_identifier"`
	CallingStation bool         `json:"calling_station" toml:"calling_station"` // send MAC as Calling-Station-Id
	SendOption82   bool         `json:"send_option82" toml:"send_option82"`     // send Option 82 attrs (NAS-Port-Id, Called-Station-Id, NAS-IP-Address)
}

// Option82Info holds DHCP relay agent (Option 82) data for RADIUS attributes.
// RFC 4014 — RADIUS Attributes Sub-Option for the DHCP Relay Agent Information Option.
type Option82Info struct {
	CircuitID string // sub-option 1: agent circuit-id → NAS-Port-Id
	RemoteID  string // sub-option 2: agent remote-id → Called-Station-Id
	GIAddr    string // relay agent IP → NAS-IP-Address
}

// AuthResult holds the result of a RADIUS authentication attempt.
type AuthResult struct {
	Accepted bool              `json:"accepted"`
	Code     string            `json:"code"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Error    string            `json:"error,omitempty"`
	Latency  float64           `json:"latency_ms"`
}

// Client handles RADIUS authentication for DHCP clients.
type Client struct {
	logger  *slog.Logger
	mu      sync.RWMutex
	subnets map[string]*SubnetConfig // subnet CIDR -> config
}

// NewClient creates a new RADIUS client.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		logger:  logger,
		subnets: make(map[string]*SubnetConfig),
	}
}

// SetSubnet configures RADIUS for a specific subnet.
func (c *Client) SetSubnet(subnet string, cfg *SubnetConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subnets[subnet] = cfg
}

// RemoveSubnet removes RADIUS configuration for a subnet.
func (c *Client) RemoveSubnet(subnet string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subnets, subnet)
}

// GetSubnet returns the RADIUS configuration for a subnet.
func (c *Client) GetSubnet(subnet string) *SubnetConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg, ok := c.subnets[subnet]
	if !ok {
		return nil
	}
	cp := *cfg
	return &cp
}

// ListSubnets returns all configured subnets.
func (c *Client) ListSubnets() map[string]SubnetConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]SubnetConfig, len(c.subnets))
	for k, v := range c.subnets {
		result[k] = *v
	}
	return result
}

// Authenticate performs RADIUS Access-Request for a DHCP client.
// The opt82 parameter may be nil if no relay agent info is present.
func (c *Client) Authenticate(ctx context.Context, subnet string, mac net.HardwareAddr, username string, opt82 *Option82Info) AuthResult {
	c.mu.RLock()
	cfg, ok := c.subnets[subnet]
	c.mu.RUnlock()

	if !ok || !cfg.Enabled {
		return AuthResult{Accepted: true, Code: "no_radius", Latency: 0}
	}

	return c.doAuth(ctx, cfg, mac, username, opt82)
}

// Test performs a test authentication against a RADIUS server.
func (c *Client) Test(ctx context.Context, cfg *ServerConfig, username, password string) AuthResult {
	subCfg := &SubnetConfig{
		Enabled:        true,
		Server:         *cfg,
		CallingStation: true,
	}
	return c.doAuthWithPassword(ctx, subCfg, username, password)
}

func (c *Client) doAuth(ctx context.Context, cfg *SubnetConfig, mac net.HardwareAddr, username string, opt82 *Option82Info) AuthResult {
	timeout := parseTimeout(cfg.Server.Timeout)

	packet := radius.New(radius.CodeAccessRequest, []byte(cfg.Server.Secret))
	rfc2865.UserName_SetString(packet, username)
	rfc2865.UserPassword_SetString(packet, mac.String()) // MAC as password
	if cfg.CallingStation && mac != nil {
		rfc2865.CallingStationID_SetString(packet, mac.String())
	}
	if cfg.NASIdentifier != "" {
		rfc2865.NASIdentifier_SetString(packet, cfg.NASIdentifier)
	}

	// Option 82 relay agent attributes (RFC 4014)
	if cfg.SendOption82 && opt82 != nil {
		if opt82.CircuitID != "" {
			// Circuit-ID → NAS-Port-Id (attribute 87, RFC 2869)
			rfc2869.NASPortID_SetString(packet, opt82.CircuitID)
		}
		if opt82.RemoteID != "" {
			// Remote-ID → Called-Station-Id (attribute 30)
			rfc2865.CalledStationID_SetString(packet, opt82.RemoteID)
		}
		if opt82.GIAddr != "" {
			// Relay agent IP → NAS-IP-Address (attribute 4)
			ip := net.ParseIP(opt82.GIAddr)
			if ip != nil {
				rfc2865.NASIPAddress_Set(packet, ip)
			}
		}
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := radius.Exchange(ctx, packet, cfg.Server.Address)
	latency := float64(time.Since(start).Microseconds()) / 1000

	if err != nil {
		c.logger.Warn("RADIUS auth failed",
			"server", cfg.Server.Address,
			"user", username,
			"error", err)
		return AuthResult{
			Accepted: false,
			Code:     "error",
			Error:    err.Error(),
			Latency:  latency,
		}
	}

	result := AuthResult{
		Accepted: resp.Code == radius.CodeAccessAccept,
		Code:     resp.Code.String(),
		Latency:  latency,
	}

	c.logger.Debug("RADIUS auth result",
		"server", cfg.Server.Address,
		"user", username,
		"accepted", result.Accepted,
		"code", result.Code,
		"latency_ms", result.Latency)

	return result
}

func (c *Client) doAuthWithPassword(ctx context.Context, cfg *SubnetConfig, username, password string) AuthResult {
	timeout := parseTimeout(cfg.Server.Timeout)

	packet := radius.New(radius.CodeAccessRequest, []byte(cfg.Server.Secret))
	rfc2865.UserName_SetString(packet, username)
	rfc2865.UserPassword_SetString(packet, password)
	if cfg.NASIdentifier != "" {
		rfc2865.NASIdentifier_SetString(packet, cfg.NASIdentifier)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := radius.Exchange(ctx, packet, cfg.Server.Address)
	latency := float64(time.Since(start).Microseconds()) / 1000

	if err != nil {
		return AuthResult{
			Accepted: false,
			Code:     "error",
			Error:    err.Error(),
			Latency:  latency,
		}
	}

	return AuthResult{
		Accepted: resp.Code == radius.CodeAccessAccept,
		Code:     resp.Code.String(),
		Latency:  latency,
	}
}

func parseTimeout(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 5 * time.Second
	}
	return d
}
