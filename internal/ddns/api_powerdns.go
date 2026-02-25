package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// PowerDNSClient performs DNS updates via the PowerDNS HTTP API.
type PowerDNSClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  *slog.Logger
}

// NewPowerDNSClient creates a new PowerDNS API client.
func NewPowerDNSClient(baseURL, apiKey string, timeout time.Duration, logger *slog.Logger) *PowerDNSClient {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &PowerDNSClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
		logger:  logger,
	}
}

// pdnsRRSet represents a PowerDNS RRSet for PATCH requests.
type pdnsRRSet struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	TTL        int          `json:"ttl"`
	Changetype string       `json:"changetype"`
	Records    []pdnsRecord `json:"records"`
}

type pdnsRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

type pdnsPatchBody struct {
	RRSets []pdnsRRSet `json:"rrsets"`
}

// AddA adds or replaces an A record.
func (c *PowerDNSClient) AddA(zone, fqdn string, ip net.IP, ttl uint32) error {
	body := pdnsPatchBody{
		RRSets: []pdnsRRSet{{
			Name:       ensureDot(fqdn),
			Type:       "A",
			TTL:        int(ttl),
			Changetype: "REPLACE",
			Records:    []pdnsRecord{{Content: ip.String(), Disabled: false}},
		}},
	}
	return c.patchZone(zone, body, "AddA", fqdn)
}

// RemoveA removes an A record.
func (c *PowerDNSClient) RemoveA(zone, fqdn string) error {
	body := pdnsPatchBody{
		RRSets: []pdnsRRSet{{
			Name:       ensureDot(fqdn),
			Type:       "A",
			Changetype: "DELETE",
			Records:    []pdnsRecord{},
		}},
	}
	return c.patchZone(zone, body, "RemoveA", fqdn)
}

// AddPTR adds or replaces a PTR record.
func (c *PowerDNSClient) AddPTR(zone, reverseIP, fqdn string, ttl uint32) error {
	body := pdnsPatchBody{
		RRSets: []pdnsRRSet{{
			Name:       ensureDot(reverseIP),
			Type:       "PTR",
			TTL:        int(ttl),
			Changetype: "REPLACE",
			Records:    []pdnsRecord{{Content: ensureDot(fqdn), Disabled: false}},
		}},
	}
	return c.patchZone(zone, body, "AddPTR", reverseIP)
}

// RemovePTR removes a PTR record.
func (c *PowerDNSClient) RemovePTR(zone, reverseIP string) error {
	body := pdnsPatchBody{
		RRSets: []pdnsRRSet{{
			Name:       ensureDot(reverseIP),
			Type:       "PTR",
			Changetype: "DELETE",
			Records:    []pdnsRecord{},
		}},
	}
	return c.patchZone(zone, body, "RemovePTR", reverseIP)
}

// patchZone sends a PATCH request to the PowerDNS API.
func (c *PowerDNSClient) patchZone(zone string, body pdnsPatchBody, op, name string) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling %s request: %w", op, err)
	}

	url := fmt.Sprintf("%s/api/v1/servers/localhost/zones/%s", c.baseURL, ensureDot(zone))
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating %s request: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("PowerDNS API request failed",
			"op", op, "name", name, "error", err, "duration", duration.String())
		return fmt.Errorf("PowerDNS %s for %s: %w", op, name, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("PowerDNS API error",
			"op", op, "name", name, "status", resp.StatusCode,
			"body", string(respBody), "duration", duration.String())
		return fmt.Errorf("PowerDNS %s for %s: HTTP %d: %s", op, name, resp.StatusCode, string(respBody))
	}

	c.logger.Debug("PowerDNS API success",
		"op", op, "name", name, "duration", duration.String())
	return nil
}
