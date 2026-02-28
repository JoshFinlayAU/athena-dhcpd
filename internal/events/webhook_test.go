package events

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookSenderBasic(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)

		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if r.Header.Get("X-Athena-Event") == "" {
			t.Error("missing X-Athena-Event header")
		}

		body, _ := io.ReadAll(r.Body)
		var evt Event
		if err := json.Unmarshal(body, &evt); err != nil {
			t.Errorf("failed to unmarshal event: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sender := NewWebhookSender(5*time.Second, logger)

	cfg := WebhookConfig{
		Name:    "test-hook",
		URL:     server.URL,
		Method:  "POST",
		Retries: 1,
	}

	evt := Event{
		Type:      EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &LeaseData{
			IP:       net.IPv4(192, 168, 1, 100),
			MAC:      "00:11:22:33:44:55",
			Hostname: "testhost",
			Subnet:   "192.168.1.0/24",
		},
	}

	sender.Send(cfg, evt)
	sender.Wait()

	if received.Load() != 1 {
		t.Errorf("webhook received %d requests, want 1", received.Load())
	}
}

func TestWebhookSenderHMAC(t *testing.T) {
	var sigHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Athena-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sender := NewWebhookSender(5*time.Second, logger)

	cfg := WebhookConfig{
		Name:    "hmac-hook",
		URL:     server.URL,
		Secret:  "test-secret",
		Retries: 1,
	}

	sender.Send(cfg, Event{Type: EventLeaseAck, Timestamp: time.Now()})
	sender.Wait()

	if sigHeader == "" {
		t.Error("expected X-Athena-Signature header when secret is set")
	}
	if len(sigHeader) < 10 || sigHeader[:7] != "sha256=" {
		t.Errorf("signature format wrong: %q", sigHeader)
	}
}

func TestWebhookSenderRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sender := NewWebhookSender(5*time.Second, logger)

	cfg := WebhookConfig{
		Name:         "retry-hook",
		URL:          server.URL,
		Retries:      3,
		RetryBackoff: 10 * time.Millisecond,
	}

	sender.Send(cfg, Event{Type: EventConflictDetected, Timestamp: time.Now()})
	sender.Wait()

	if attempts.Load() != 3 {
		t.Errorf("webhook attempts = %d, want 3", attempts.Load())
	}
}

func TestWebhookSenderCustomHeaders(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sender := NewWebhookSender(5*time.Second, logger)

	cfg := WebhookConfig{
		Name:    "header-hook",
		URL:     server.URL,
		Headers: map[string]string{"Authorization": "Bearer test-token"},
		Retries: 1,
	}

	sender.Send(cfg, Event{Type: EventLeaseAck, Timestamp: time.Now()})
	sender.Wait()

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
}

func TestSlackPayload(t *testing.T) {
	evt := Event{
		Type:      EventConflictDetected,
		Timestamp: time.Now(),
		Conflict: &ConflictData{
			IP:              net.IPv4(192, 168, 1, 100),
			DetectionMethod: "arp_probe",
			ResponderMAC:    "aa:bb:cc:dd:ee:ff",
		},
	}

	body, err := buildSlackPayload(evt)
	if err != nil {
		t.Fatalf("buildSlackPayload error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if payload["text"] == "" {
		t.Error("slack payload text is empty")
	}
}

func TestTeamsPayload(t *testing.T) {
	evt := Event{
		Type:      EventLeaseAck,
		Timestamp: time.Now(),
		Lease: &LeaseData{
			IP:       net.IPv4(192, 168, 1, 100),
			Hostname: "testhost",
		},
	}

	body, err := buildTeamsPayload(evt)
	if err != nil {
		t.Fatalf("buildTeamsPayload error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if payload["@type"] != "MessageCard" {
		t.Errorf("@type = %v, want MessageCard", payload["@type"])
	}
}

func TestComputeHMAC(t *testing.T) {
	sig := computeHMAC([]byte("test-payload"), "test-secret")
	if sig == "" {
		t.Error("HMAC signature is empty")
	}
	if len(sig) != 64 { // SHA256 hex = 64 chars
		t.Errorf("HMAC signature length = %d, want 64", len(sig))
	}

	// Same input should produce same output
	sig2 := computeHMAC([]byte("test-payload"), "test-secret")
	if sig != sig2 {
		t.Error("HMAC not deterministic")
	}

	// Different secret should produce different output
	sig3 := computeHMAC([]byte("test-payload"), "different-secret")
	if sig == sig3 {
		t.Error("different secrets produced same HMAC")
	}
}
