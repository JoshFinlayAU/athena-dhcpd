package events

import (
	"log/slog"
	"strings"
	"time"
)

// Dispatcher routes events from the bus to script hooks and webhooks.
// It subscribes to the event bus and dispatches matching events to the
// appropriate hook runners. Hook failures NEVER propagate to DHCP processing.
type Dispatcher struct {
	bus      *Bus
	scripts  *ScriptRunner
	webhooks *WebhookSender
	logger   *slog.Logger
	scriptCfgs  []ScriptConfig
	webhookCfgs []WebhookConfig
	ch          chan Event
	done        chan struct{}
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher(bus *Bus, logger *slog.Logger, scriptConcurrency int, webhookTimeout time.Duration) *Dispatcher {
	return &Dispatcher{
		bus:      bus,
		scripts:  NewScriptRunner(scriptConcurrency, logger),
		webhooks: NewWebhookSender(webhookTimeout, logger),
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// AddScript registers a script hook.
func (d *Dispatcher) AddScript(cfg ScriptConfig) {
	d.scriptCfgs = append(d.scriptCfgs, cfg)
}

// AddWebhook registers a webhook hook.
func (d *Dispatcher) AddWebhook(cfg WebhookConfig) {
	d.webhookCfgs = append(d.webhookCfgs, cfg)
}

// Start subscribes to the event bus and begins dispatching. Call in a goroutine.
func (d *Dispatcher) Start() {
	d.ch = d.bus.Subscribe(1000)

	d.logger.Info("event dispatcher started",
		"script_hooks", len(d.scriptCfgs),
		"webhook_hooks", len(d.webhookCfgs))

	for {
		select {
		case evt, ok := <-d.ch:
			if !ok {
				return
			}
			d.dispatch(evt)
		case <-d.done:
			return
		}
	}
}

// Stop shuts down the dispatcher and waits for pending hooks.
func (d *Dispatcher) Stop() {
	close(d.done)
	if d.ch != nil {
		d.bus.Unsubscribe(d.ch)
	}
	d.scripts.Wait()
	d.webhooks.Wait()
	d.logger.Info("event dispatcher stopped")
}

// dispatch routes a single event to matching hooks.
func (d *Dispatcher) dispatch(evt Event) {
	evtType := string(evt.Type)

	for _, cfg := range d.scriptCfgs {
		if matchesEvent(cfg.Events, evtType) && matchesSubnet(cfg.Subnets, evt) {
			d.scripts.Run(cfg, evt)
		}
	}

	for _, cfg := range d.webhookCfgs {
		if matchesEvent(cfg.Events, evtType) {
			d.webhooks.Send(cfg, evt)
		}
	}
}

// matchesEvent checks if the event type matches any of the configured patterns.
// Supports exact match and wildcard patterns (e.g., "lease.*", "*").
func matchesEvent(patterns []string, eventType string) bool {
	if len(patterns) == 0 {
		return true // No filter = match all
	}
	for _, p := range patterns {
		if p == "*" {
			return true
		}
		if p == eventType {
			return true
		}
		// Wildcard suffix: "lease.*" matches "lease.ack", "lease.release", etc.
		if strings.HasSuffix(p, ".*") {
			prefix := strings.TrimSuffix(p, ".*")
			if strings.HasPrefix(eventType, prefix+".") {
				return true
			}
		}
	}
	return false
}

// matchesSubnet checks if the event's subnet matches the hook's subnet filter.
func matchesSubnet(subnets []string, evt Event) bool {
	if len(subnets) == 0 {
		return true // No filter = match all
	}

	var evtSubnet string
	if evt.Lease != nil {
		evtSubnet = evt.Lease.Subnet
	}
	if evt.Conflict != nil && evtSubnet == "" {
		evtSubnet = evt.Conflict.Subnet
	}

	if evtSubnet == "" {
		return true // No subnet in event = match all
	}

	for _, s := range subnets {
		if s == evtSubnet {
			return true
		}
	}
	return false
}
