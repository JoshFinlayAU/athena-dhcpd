// Package fingerprint — Fingerbank API client for enhanced device classification.
// Uses the Fingerbank v2 API (api.fingerbank.org) to look up device information
// from DHCP fingerprint data (option 55 parameter request list + option 60 vendor class).
package fingerprint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultFingerbankURL = "https://api.fingerbank.org/api/v2"
	fingerbankTimeout    = 10 * time.Second
)

// FingerbankClient queries the Fingerbank API for device classification.
type FingerbankClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
	logger  *slog.Logger

	// Simple rate limiter: max 1 request per 100ms to be polite
	mu       sync.Mutex
	lastCall time.Time
}

// NewFingerbankClient creates a new Fingerbank API client.
// Returns nil if apiKey is empty.
func NewFingerbankClient(apiKey, baseURL string, logger *slog.Logger) *FingerbankClient {
	if apiKey == "" {
		return nil
	}
	if baseURL == "" {
		baseURL = defaultFingerbankURL
	}
	return &FingerbankClient{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: fingerbankTimeout},
		logger:  logger,
	}
}

// fingerbankResponse is the JSON response from the Fingerbank /combinations/interrogate endpoint.
type fingerbankResponse struct {
	Device struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Parent struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"parent"`
	} `json:"device"`
	DeviceName string  `json:"device_name"`
	Score      int     `json:"score"`
	Version    string  `json:"version"`
	Confidence float64 `json:"confidence"`
}

// Classify queries the Fingerbank API with DHCP fingerprint data and enriches the DeviceInfo.
// This is called asynchronously — it should not block the DHCP response path.
func (fb *FingerbankClient) Classify(ctx context.Context, info *DeviceInfo, fp *RawFingerprint) error {
	if fb == nil {
		return nil
	}

	// Simple rate limiting
	fb.mu.Lock()
	since := time.Since(fb.lastCall)
	if since < 100*time.Millisecond {
		fb.mu.Unlock()
		time.Sleep(100*time.Millisecond - since)
		fb.mu.Lock()
	}
	fb.lastCall = time.Now()
	fb.mu.Unlock()

	// Build query parameters
	params := url.Values{}
	params.Set("key", fb.apiKey)

	// DHCP fingerprint = comma-separated option 55 parameter request list
	if len(fp.ParamList) > 0 {
		params.Set("dhcp_fingerprint", fp.ParamListString())
	}

	// Vendor class (option 60)
	if fp.VendorClass != "" {
		params.Set("dhcp_vendor", fp.VendorClass)
	}

	// Hostname (option 12)
	if fp.Hostname != "" {
		params.Set("computer_name", fp.Hostname)
	}

	// MAC for OUI lookup
	if fp.MAC != nil {
		params.Set("mac", fp.MAC.String())
	}

	reqURL := fmt.Sprintf("%s/combinations/interrogate?%s", fb.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating fingerbank request: %w", err)
	}

	resp, err := fb.client.Do(req)
	if err != nil {
		return fmt.Errorf("fingerbank API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("fingerbank API: invalid API key")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("fingerbank API: rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("fingerbank API: status %d: %s", resp.StatusCode, string(body))
	}

	var fbResp fingerbankResponse
	if err := json.NewDecoder(resp.Body).Decode(&fbResp); err != nil {
		return fmt.Errorf("decoding fingerbank response: %w", err)
	}

	// Enrich the device info with Fingerbank data
	if fbResp.Device.Name != "" {
		info.DeviceName = fbResp.Device.Name
		info.Source = "fingerbank"
		if fbResp.Score > 0 {
			info.Confidence = min(fbResp.Score, 100)
		}

		// Try to extract device type from parent category
		parentName := strings.ToLower(fbResp.Device.Parent.Name)
		switch {
		case strings.Contains(parentName, "phone"), strings.Contains(parentName, "smartphone"):
			info.DeviceType = "phone"
		case strings.Contains(parentName, "tablet"):
			info.DeviceType = "phone"
		case strings.Contains(parentName, "computer"), strings.Contains(parentName, "laptop"),
			strings.Contains(parentName, "desktop"), strings.Contains(parentName, "workstation"):
			info.DeviceType = "computer"
		case strings.Contains(parentName, "printer"):
			info.DeviceType = "printer"
		case strings.Contains(parentName, "switch"), strings.Contains(parentName, "router"),
			strings.Contains(parentName, "access point"), strings.Contains(parentName, "network"):
			info.DeviceType = "network"
		case strings.Contains(parentName, "camera"), strings.Contains(parentName, "surveillance"):
			info.DeviceType = "camera"
		case strings.Contains(parentName, "iot"), strings.Contains(parentName, "sensor"),
			strings.Contains(parentName, "thermostat"), strings.Contains(parentName, "smart"):
			info.DeviceType = "embedded"
		case strings.Contains(parentName, "gaming"), strings.Contains(parentName, "console"):
			info.DeviceType = "gaming"
		case strings.Contains(parentName, "tv"), strings.Contains(parentName, "media"),
			strings.Contains(parentName, "streaming"):
			info.DeviceType = "media"
		}

		// Extract OS from device name
		deviceLower := strings.ToLower(fbResp.Device.Name)
		switch {
		case strings.Contains(deviceLower, "windows"):
			info.OS = "Windows"
		case strings.Contains(deviceLower, "macos"), strings.Contains(deviceLower, "mac os"):
			info.OS = "macOS"
		case strings.Contains(deviceLower, "ios"), strings.Contains(deviceLower, "iphone"),
			strings.Contains(deviceLower, "ipad"):
			info.OS = "iOS/iPadOS"
		case strings.Contains(deviceLower, "android"):
			info.OS = "Android"
		case strings.Contains(deviceLower, "linux"), strings.Contains(deviceLower, "ubuntu"),
			strings.Contains(deviceLower, "debian"):
			info.OS = "Linux"
		case strings.Contains(deviceLower, "chrome"):
			info.OS = "ChromeOS"
		}
	}

	fb.logger.Debug("fingerbank classification",
		"mac", info.MAC,
		"device", fbResp.Device.Name,
		"parent", fbResp.Device.Parent.Name,
		"score", fbResp.Score)

	return nil
}
