package ddns

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"
)

// TechnitiumClient performs DNS updates via the Technitium DNS Server HTTP API.
type TechnitiumClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  *slog.Logger
}

// NewTechnitiumClient creates a new Technitium DNS API client.
func NewTechnitiumClient(baseURL, apiKey string, timeout time.Duration, logger *slog.Logger) *TechnitiumClient {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &TechnitiumClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
		logger:  logger,
	}
}

// AddA adds or updates an A record.
func (c *TechnitiumClient) AddA(zone, fqdn string, ip net.IP, ttl uint32) error {
	params := url.Values{
		"token":    {c.apiKey},
		"domain":   {fqdn},
		"zone":     {zone},
		"type":     {"A"},
		"ipAddress": {ip.String()},
		"ttl":      {fmt.Sprintf("%d", ttl)},
		"overwrite": {"true"},
	}
	return c.doRequest("/api/zones/records/add", params, "AddA", fqdn)
}

// RemoveA removes an A record.
func (c *TechnitiumClient) RemoveA(zone, fqdn string) error {
	params := url.Values{
		"token":  {c.apiKey},
		"domain": {fqdn},
		"zone":   {zone},
		"type":   {"A"},
	}
	return c.doRequest("/api/zones/records/delete", params, "RemoveA", fqdn)
}

// AddPTR adds or updates a PTR record.
func (c *TechnitiumClient) AddPTR(zone, reverseIP, fqdn string, ttl uint32) error {
	params := url.Values{
		"token":     {c.apiKey},
		"domain":    {reverseIP},
		"zone":      {zone},
		"type":      {"PTR"},
		"ptrName":   {fqdn},
		"ttl":       {fmt.Sprintf("%d", ttl)},
		"overwrite": {"true"},
	}
	return c.doRequest("/api/zones/records/add", params, "AddPTR", reverseIP)
}

// RemovePTR removes a PTR record.
func (c *TechnitiumClient) RemovePTR(zone, reverseIP string) error {
	params := url.Values{
		"token":  {c.apiKey},
		"domain": {reverseIP},
		"zone":   {zone},
		"type":   {"PTR"},
	}
	return c.doRequest("/api/zones/records/delete", params, "RemovePTR", reverseIP)
}

// doRequest performs an HTTP GET request to the Technitium API.
func (c *TechnitiumClient) doRequest(path string, params url.Values, op, name string) error {
	reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, path, params.Encode())

	start := time.Now()
	resp, err := c.client.Get(reqURL)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("Technitium API request failed",
			"op", op, "name", name, "error", err, "duration", duration.String())
		return fmt.Errorf("Technitium %s for %s: %w", op, name, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("Technitium API error",
			"op", op, "name", name, "status", resp.StatusCode,
			"body", string(respBody), "duration", duration.String())
		return fmt.Errorf("Technitium %s for %s: HTTP %d: %s", op, name, resp.StatusCode, string(respBody))
	}

	c.logger.Debug("Technitium API success",
		"op", op, "name", name, "duration", duration.String())
	return nil
}
