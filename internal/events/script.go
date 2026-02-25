package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// ScriptRunner executes script hooks in a bounded goroutine pool.
// This is the ONLY permitted use of os/exec in the entire project.
type ScriptRunner struct {
	logger      *slog.Logger
	concurrency int
	sem         chan struct{} // Semaphore for bounding concurrency
	wg          sync.WaitGroup
}

// ScriptConfig describes a single script hook binding.
type ScriptConfig struct {
	Name    string
	Events  []string
	Command string
	Timeout time.Duration
	Subnets []string // Optional subnet filter
}

// NewScriptRunner creates a new script runner with the given concurrency limit.
func NewScriptRunner(concurrency int, logger *slog.Logger) *ScriptRunner {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &ScriptRunner{
		logger:      logger,
		concurrency: concurrency,
		sem:         make(chan struct{}, concurrency),
	}
}

// Run executes a script hook for the given event. Non-blocking — runs in a goroutine.
// Script receives event data via environment variables (ATHENA_* prefix) AND JSON on stdin.
func (r *ScriptRunner) Run(cfg ScriptConfig, evt Event) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		// Acquire semaphore slot
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		default:
			r.logger.Warn("script hook pool full, dropping execution",
				"hook_name", cfg.Name,
				"event", string(evt.Type))
			return
		}

		r.execute(cfg, evt)
	}()
}

// execute runs a single script with timeout, env vars, and JSON stdin.
func (r *ScriptRunner) execute(cfg ScriptConfig, evt Event) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cfg.Command)

	// Set environment variables from event
	envVars := evt.ToEnvVars()
	envVars["ATHENA_HOOK_NAME"] = cfg.Name
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Pass JSON on stdin
	jsonData, err := json.Marshal(evt)
	if err != nil {
		r.logger.Error("failed to marshal event for script stdin",
			"hook_name", cfg.Name,
			"error", err)
		return
	}
	cmd.Stdin = bytes.NewReader(jsonData)

	// Capture stdout/stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	err = cmd.Run()
	duration := time.Since(start)

	if err != nil {
		metrics.HookExecutions.WithLabelValues("script", "error").Inc()
		metrics.HookDuration.WithLabelValues("script").Observe(duration.Seconds())
		if ctx.Err() == context.DeadlineExceeded {
			r.logger.Error("script hook timed out — killed",
				"hook_name", cfg.Name,
				"command", cfg.Command,
				"timeout", timeout.String(),
				"event", string(evt.Type))
		} else {
			r.logger.Error("script hook failed",
				"hook_name", cfg.Name,
				"command", cfg.Command,
				"error", err,
				"stderr", stderr.String(),
				"duration", duration.String(),
				"event", string(evt.Type))
		}
		return
	}

	metrics.HookExecutions.WithLabelValues("script", "success").Inc()
	metrics.HookDuration.WithLabelValues("script").Observe(duration.Seconds())

	r.logger.Debug("script hook completed",
		"hook_name", cfg.Name,
		"duration", duration.String(),
		"event", string(evt.Type),
		"exit_code", cmd.ProcessState.ExitCode())
}

// Wait blocks until all running scripts complete.
func (r *ScriptRunner) Wait() {
	r.wg.Wait()
}
