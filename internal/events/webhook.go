package events

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// WebhookSender sends events to webhook endpoints with retry and HMAC signing.
type WebhookSender struct {
	client *http.Client
	logger *slog.Logger
	wg     sync.WaitGroup
}

// WebhookConfig describes a single webhook binding.
type WebhookConfig struct {
	Name         string
	Events       []string
	URL          string
	Method       string
	Headers      map[string]string
	Timeout      time.Duration
	Retries      int
	RetryBackoff time.Duration
	Secret       string // HMAC secret for signing
	Template     string // "slack", "teams", or empty for raw JSON
}

// NewWebhookSender creates a new webhook sender with a shared HTTP client pool.
func NewWebhookSender(timeout time.Duration, logger *slog.Logger) *WebhookSender {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &WebhookSender{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger: logger,
	}
}

// Send sends an event to a webhook endpoint. Non-blocking — runs in a goroutine.
func (w *WebhookSender) Send(cfg WebhookConfig, evt Event) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.sendWithRetry(cfg, evt)
	}()
}

// sendWithRetry attempts to deliver the webhook with exponential backoff.
func (w *WebhookSender) sendWithRetry(cfg WebhookConfig, evt Event) {
	var body []byte
	var err error

	switch cfg.Template {
	case "slack":
		body, err = buildSlackPayload(evt)
	case "teams":
		body, err = buildTeamsPayload(evt)
	default:
		body, err = json.Marshal(evt)
	}
	if err != nil {
		w.logger.Error("failed to marshal webhook payload",
			"hook_name", cfg.Name,
			"error", err)
		return
	}

	method := cfg.Method
	if method == "" {
		method = "POST"
	}

	retries := cfg.Retries
	if retries <= 0 {
		retries = 1
	}
	backoff := cfg.RetryBackoff
	if backoff == 0 {
		backoff = time.Second
	}

	start := time.Now()

	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			sleepDuration := backoff * time.Duration(1<<uint(attempt-1))
			time.Sleep(sleepDuration)
		}

		err = w.doRequest(cfg, method, body)
		if err == nil {
			metrics.HookExecutions.WithLabelValues("webhook", "success").Inc()
			metrics.HookDuration.WithLabelValues("webhook").Observe(time.Since(start).Seconds())
			w.logger.Debug("webhook delivered",
				"hook_name", cfg.Name,
				"url", cfg.URL,
				"event", string(evt.Type),
				"attempt", attempt+1)
			return
		}

		w.logger.Warn("webhook delivery failed, retrying",
			"hook_name", cfg.Name,
			"url", cfg.URL,
			"attempt", attempt+1,
			"max_retries", retries,
			"error", err)
	}

	metrics.HookExecutions.WithLabelValues("webhook", "error").Inc()
	metrics.HookDuration.WithLabelValues("webhook").Observe(time.Since(start).Seconds())

	w.logger.Error("webhook delivery failed after all retries",
		"hook_name", cfg.Name,
		"url", cfg.URL,
		"retries", retries,
		"error", err)
}

// doRequest performs a single HTTP request.
func (w *WebhookSender) doRequest(cfg WebhookConfig, method string, body []byte) error {
	req, err := http.NewRequest(method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Athena-Event", "dhcp-event")
	req.Header.Set("User-Agent", "athena-dhcpd/1.0")

	// Custom headers
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// HMAC signature
	if cfg.Secret != "" {
		sig := computeHMAC(body, cfg.Secret)
		req.Header.Set("X-Athena-Signature", "sha256="+sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request to %s: %w", cfg.URL, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
}

// computeHMAC computes HMAC-SHA256 of the payload.
func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Wait blocks until all pending webhooks complete.
func (w *WebhookSender) Wait() {
	w.wg.Wait()
}

// buildSlackPayload creates a Slack-formatted webhook payload.
func buildSlackPayload(evt Event) ([]byte, error) {
	text := fmt.Sprintf("*%s*", evt.Type)
	if evt.Lease != nil {
		if evt.Lease.IP != nil {
			text += fmt.Sprintf("\nIP: `%s`", evt.Lease.IP)
		}
		if evt.Lease.MAC != nil {
			text += fmt.Sprintf("\nMAC: `%s`", evt.Lease.MAC)
		}
		if evt.Lease.Hostname != "" {
			text += fmt.Sprintf("\nHostname: `%s`", evt.Lease.Hostname)
		}
		if evt.Lease.Subnet != "" {
			text += fmt.Sprintf("\nSubnet: `%s`", evt.Lease.Subnet)
		}
	}
	if evt.Conflict != nil {
		if evt.Conflict.IP != nil {
			text += fmt.Sprintf("\nConflict IP: `%s`", evt.Conflict.IP)
		}
		text += fmt.Sprintf("\nMethod: `%s`", evt.Conflict.DetectionMethod)
		if evt.Conflict.ResponderMAC != "" {
			text += fmt.Sprintf("\nResponder: `%s`", evt.Conflict.ResponderMAC)
		}
	}
	if evt.Reason != "" {
		text += fmt.Sprintf("\nReason: %s", evt.Reason)
	}

	payload := map[string]string{"text": text}
	return json.Marshal(payload)
}

// buildTeamsPayload creates a Microsoft Teams-formatted webhook payload.
func buildTeamsPayload(evt Event) ([]byte, error) {
	title := string(evt.Type)
	text := fmt.Sprintf("Event: **%s** at %s", evt.Type, evt.Timestamp.Format(time.RFC3339))

	if evt.Lease != nil {
		if evt.Lease.IP != nil {
			text += fmt.Sprintf("<br>IP: %s", evt.Lease.IP)
		}
		if evt.Lease.MAC != nil {
			text += fmt.Sprintf("<br>MAC: %s", evt.Lease.MAC)
		}
		if evt.Lease.Hostname != "" {
			text += fmt.Sprintf("<br>Hostname: %s", evt.Lease.Hostname)
		}
	}
	if evt.Conflict != nil {
		if evt.Conflict.IP != nil {
			text += fmt.Sprintf("<br>Conflict IP: %s", evt.Conflict.IP)
		}
		text += fmt.Sprintf("<br>Detection: %s", evt.Conflict.DetectionMethod)
	}

	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"summary":    title,
		"themeColor": "0076D7",
		"title":      "athena-dhcpd: " + title,
		"text":       text,
	}
	return json.Marshal(payload)
}
