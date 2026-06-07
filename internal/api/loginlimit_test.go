package api

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoginLimiterLocksAfterMaxFailures(t *testing.T) {
	l := newLoginLimiter(3, time.Minute, 5*time.Minute)
	now := time.Now()
	key := "10.0.0.1"

	for i := 0; i < 2; i++ {
		l.recordFailure(key, now)
		if locked, _ := l.locked(key, now); locked {
			t.Fatalf("locked after %d failures, want unlocked (< max)", i+1)
		}
	}

	// Third failure trips the lockout.
	l.recordFailure(key, now)
	locked, retry := l.locked(key, now)
	if !locked {
		t.Fatal("expected lockout after max failures")
	}
	if retry <= 0 || retry > 5*time.Minute {
		t.Fatalf("retry-after = %v, want within (0, 5m]", retry)
	}

	// Lockout clears after the lockout window passes.
	if locked, _ := l.locked(key, now.Add(5*time.Minute+time.Second)); locked {
		t.Fatal("expected lockout to expire")
	}
}

func TestLoginLimiterResetOnSuccess(t *testing.T) {
	l := newLoginLimiter(2, time.Minute, time.Minute)
	now := time.Now()
	key := "10.0.0.2"

	l.recordFailure(key, now)
	l.reset(key)
	l.recordFailure(key, now)
	if locked, _ := l.locked(key, now); locked {
		t.Fatal("reset should have cleared the prior failure count")
	}
}

func TestOpenAccessFailsClosedAfterSetup(t *testing.T) {
	// No credentials configured.
	a := NewAuthMiddleware(config.APIConfig{}, discardLogger())

	// Before setup completes: open (so the wizard can run).
	if !a.openAccess() {
		t.Fatal("expected open access before setup completes")
	}

	// After setup completes with still no credentials: fail closed.
	a.setupComplete = func() bool { return true }
	if a.openAccess() {
		t.Fatal("expected fail-closed once setup is complete and no credentials exist")
	}

	// With a bearer token configured, never open regardless of setup state.
	a.bearerToken = "secret"
	if a.openAccess() {
		t.Fatal("expected no open access when a credential is configured")
	}
}
