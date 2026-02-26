// Package portauto provides DHCP-driven switch port automation.
// It evaluates rules based on MAC address, option 82 data, fingerprint,
// and subnet to trigger actions like VLAN assignment, quarantine, or
// webhook notifications. This acts as a lightweight NAC system.
package portauto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Action defines what to do when a rule matches.
type Action struct {
	Type    string            `json:"type"`    // "webhook", "log", "tag"
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Tag     string            `json:"tag,omitempty"` // for tag actions
	VLAN    int               `json:"vlan,omitempty"`
}

// Rule defines a matching condition and action.
type Rule struct {
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Priority    int      `json:"priority"`
	MACPatterns []string `json:"mac_patterns,omitempty"` // regex patterns
	Subnets     []string `json:"subnets,omitempty"`
	CircuitIDs  []string `json:"circuit_ids,omitempty"`  // regex patterns
	RemoteIDs   []string `json:"remote_ids,omitempty"`   // regex patterns
	DeviceTypes []string `json:"device_types,omitempty"` // from fingerprinting
	Actions     []Action `json:"actions"`

	// Compiled regexes (not serialized)
	macRE     []*regexp.Regexp
	circuitRE []*regexp.Regexp
	remoteRE  []*regexp.Regexp
}

// LeaseContext holds the data available for rule evaluation.
type LeaseContext struct {
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	Subnet     string `json:"subnet"`
	CircuitID  string `json:"circuit_id"`
	RemoteID   string `json:"remote_id"`
	DeviceType string `json:"device_type"`
	Vendor     string `json:"vendor"`
}

// MatchResult holds the result of evaluating rules against a lease.
type MatchResult struct {
	Rule    string   `json:"rule"`
	Actions []Action `json:"actions"`
}

// Engine evaluates port automation rules against lease events.
type Engine struct {
	mu     sync.RWMutex
	rules  []Rule
	logger *slog.Logger
	client *http.Client
}

// NewEngine creates a new port automation engine.
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetRules replaces all rules. Compiles regex patterns.
func (e *Engine) SetRules(rules []Rule) error {
	compiled := make([]Rule, len(rules))
	for i, r := range rules {
		compiled[i] = r
		for _, p := range r.MACPatterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("invalid MAC pattern %q in rule %q: %w", p, r.Name, err)
			}
			compiled[i].macRE = append(compiled[i].macRE, re)
		}
		for _, p := range r.CircuitIDs {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("invalid circuit-id pattern %q in rule %q: %w", p, r.Name, err)
			}
			compiled[i].circuitRE = append(compiled[i].circuitRE, re)
		}
		for _, p := range r.RemoteIDs {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("invalid remote-id pattern %q in rule %q: %w", p, r.Name, err)
			}
			compiled[i].remoteRE = append(compiled[i].remoteRE, re)
		}
	}

	e.mu.Lock()
	e.rules = compiled
	e.mu.Unlock()
	return nil
}

// GetRules returns all rules (without compiled regexes).
func (e *Engine) GetRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Rule, len(e.rules))
	for i, r := range e.rules {
		cp := r
		cp.macRE = nil
		cp.circuitRE = nil
		cp.remoteRE = nil
		result[i] = cp
	}
	return result
}

// Evaluate checks all rules against a lease context and returns matches.
func (e *Engine) Evaluate(ctx LeaseContext) []MatchResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []MatchResult
	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}
		if e.matches(&r, ctx) {
			results = append(results, MatchResult{
				Rule:    r.Name,
				Actions: r.Actions,
			})
		}
	}
	return results
}

// Execute evaluates rules and executes all matching actions.
func (e *Engine) Execute(ctx LeaseContext) []MatchResult {
	matches := e.Evaluate(ctx)
	for _, m := range matches {
		for _, a := range m.Actions {
			e.executeAction(a, ctx, m.Rule)
		}
	}
	return matches
}

func (e *Engine) matches(r *Rule, ctx LeaseContext) bool {
	// All non-empty criteria must match (AND logic)
	if len(r.macRE) > 0 {
		if !matchesAny(r.macRE, ctx.MAC) {
			return false
		}
	}
	if len(r.Subnets) > 0 {
		if !containsStr(r.Subnets, ctx.Subnet) {
			return false
		}
	}
	if len(r.circuitRE) > 0 {
		if !matchesAny(r.circuitRE, ctx.CircuitID) {
			return false
		}
	}
	if len(r.remoteRE) > 0 {
		if !matchesAny(r.remoteRE, ctx.RemoteID) {
			return false
		}
	}
	if len(r.DeviceTypes) > 0 {
		if !containsStrCI(r.DeviceTypes, ctx.DeviceType) {
			return false
		}
	}
	return true
}

func (e *Engine) executeAction(a Action, ctx LeaseContext, ruleName string) {
	switch a.Type {
	case "webhook":
		e.fireWebhook(a, ctx, ruleName)
	case "log":
		e.logger.Info("port-auto rule matched",
			"rule", ruleName,
			"mac", ctx.MAC,
			"ip", ctx.IP,
			"subnet", ctx.Subnet)
	case "tag":
		e.logger.Info("port-auto tag applied",
			"rule", ruleName,
			"tag", a.Tag,
			"vlan", a.VLAN,
			"mac", ctx.MAC)
	}
}

func (e *Engine) fireWebhook(a Action, ctx LeaseContext, ruleName string) {
	payload := map[string]interface{}{
		"rule":        ruleName,
		"action":      a.Type,
		"mac":         ctx.MAC,
		"ip":          ctx.IP,
		"hostname":    ctx.Hostname,
		"subnet":      ctx.Subnet,
		"circuit_id":  ctx.CircuitID,
		"remote_id":   ctx.RemoteID,
		"device_type": ctx.DeviceType,
		"vendor":      ctx.Vendor,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	if a.VLAN > 0 {
		payload["vlan"] = a.VLAN
	}
	if a.Tag != "" {
		payload["tag"] = a.Tag
	}

	body, _ := json.Marshal(payload)
	method := a.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, a.URL, bytes.NewReader(body))
	if err != nil {
		e.logger.Warn("port-auto webhook request failed", "error", err, "url", a.URL)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Warn("port-auto webhook failed", "error", err, "url", a.URL, "rule", ruleName)
		return
	}
	resp.Body.Close()
	e.logger.Debug("port-auto webhook fired", "url", a.URL, "status", resp.StatusCode, "rule", ruleName)
}

func matchesAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func containsStrCI(list []string, s string) bool {
	lower := strings.ToLower(s)
	for _, item := range list {
		if strings.ToLower(item) == lower {
			return true
		}
	}
	return false
}
