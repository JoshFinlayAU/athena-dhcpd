// Package vip manages floating virtual IP addresses for HA failover.
// The active node holds the VIPs; on failover the new active acquires them
// and the old active releases them. Uses `ip addr` commands on Linux.
// Falls back to sudo if direct execution fails with permission errors.
package vip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// runCmd tries to run a command directly. If it fails with a permission
// error, it retries with sudo. This handles the case where CAP_NET_ADMIN
// is not set on the binary but the user has passwordless sudo configured.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "Operation not permitted") || strings.Contains(outStr, "EPERM") {
			// Retry with sudo
			sudoArgs := append([]string{name}, args...)
			return exec.CommandContext(ctx, "sudo", sudoArgs...).CombinedOutput()
		}
	}
	return out, err
}

// Entry is a single VIP configuration as stored in the database.
type Entry struct {
	IP        string `json:"ip"`
	CIDR      int    `json:"cidr"`
	Interface string `json:"interface"`
	Label     string `json:"label,omitempty"`
}

// EntryStatus is the runtime state of a single VIP for API reporting.
type EntryStatus struct {
	IP         string    `json:"ip"`
	CIDR       int       `json:"cidr"`
	Interface  string    `json:"interface"`
	Label      string    `json:"label,omitempty"`
	Held       bool      `json:"held"`
	OnLocal    bool      `json:"on_local"`
	AcquiredAt time.Time `json:"acquired_at,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// GroupStatus is the full VIP status for the HA status API.
type GroupStatus struct {
	Configured bool          `json:"configured"`
	Active     bool          `json:"active"`
	Entries    []EntryStatus `json:"entries,omitempty"`
}

// ParseEntries deserialises VIP entries from the dbconfig raw JSON.
func ParseEntries(data json.RawMessage) ([]Entry, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing VIP entries: %w", err)
	}
	return entries, nil
}

// Group manages a set of floating VIPs. Thread-safe.
type Group struct {
	mu      sync.RWMutex
	entries []entry
	active  bool
	logger  *slog.Logger
}

type entry struct {
	cfg       Entry
	ip        net.IP
	held      bool
	acquireAt time.Time
	lastErr   string
}

// NewGroup creates a VIP group from config entries.
func NewGroup(cfgs []Entry, logger *slog.Logger) (*Group, error) {
	g := &Group{logger: logger}
	for _, c := range cfgs {
		ip := net.ParseIP(c.IP)
		if ip == nil {
			return nil, fmt.Errorf("invalid VIP address: %s", c.IP)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("only IPv4 VIPs supported: %s", c.IP)
		}
		if c.CIDR <= 0 || c.CIDR > 32 {
			return nil, fmt.Errorf("invalid CIDR for VIP %s: %d", c.IP, c.CIDR)
		}
		if c.Interface == "" {
			return nil, fmt.Errorf("VIP %s: interface must not be empty", c.IP)
		}
		g.entries = append(g.entries, entry{cfg: c, ip: ip.To4()})
	}
	return g, nil
}

// Reload replaces the VIP entry list. Releases any VIPs that are no longer
// in the new list, acquires new ones if currently active.
func (g *Group) Reload(cfgs []Entry) error {
	newGroup, err := NewGroup(cfgs, g.logger)
	if err != nil {
		return err
	}

	g.mu.Lock()
	wasActive := g.active
	oldEntries := g.entries
	g.entries = newGroup.entries
	g.mu.Unlock()

	// Release VIPs from old set that aren't in the new set
	newSet := make(map[string]bool)
	for _, e := range cfgs {
		newSet[e.IP+"/"+e.Interface] = true
	}
	for _, old := range oldEntries {
		key := old.cfg.IP + "/" + old.cfg.Interface
		if old.held && !newSet[key] {
			g.releaseOne(&old)
		}
	}

	// If we're active, acquire any new VIPs
	if wasActive {
		g.mu.Lock()
		g.active = true
		g.mu.Unlock()
		g.AcquireAll()
	}
	return nil
}

// AcquireAll adds all VIPs to their interfaces and sends gratuitous ARPs.
func (g *Group) AcquireAll() {
	g.mu.Lock()
	g.active = true
	entries := g.entries
	g.mu.Unlock()

	for i := range entries {
		if err := g.acquireOne(&entries[i]); err != nil {
			g.logger.Error("failed to acquire VIP", "vip", entries[i].cfg.IP,
				"interface", entries[i].cfg.Interface, "error", err)
			g.mu.Lock()
			g.entries[i].lastErr = err.Error()
			g.mu.Unlock()
		}
	}
}

// ReleaseAll removes all VIPs from their interfaces.
func (g *Group) ReleaseAll() {
	g.mu.Lock()
	g.active = false
	entries := g.entries
	g.mu.Unlock()

	for i := range entries {
		if err := g.releaseOne(&entries[i]); err != nil {
			g.logger.Error("failed to release VIP", "vip", entries[i].cfg.IP,
				"interface", entries[i].cfg.Interface, "error", err)
		}
	}
}

// Status returns the current state of all VIPs for API reporting.
func (g *Group) Status() GroupStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	st := GroupStatus{
		Configured: len(g.entries) > 0,
		Active:     g.active,
	}
	for _, e := range g.entries {
		es := EntryStatus{
			IP:         e.cfg.IP,
			CIDR:       e.cfg.CIDR,
			Interface:  e.cfg.Interface,
			Label:      e.cfg.Label,
			Held:       e.held,
			OnLocal:    isIPOnInterface(e.ip, e.cfg.Interface),
			AcquiredAt: e.acquireAt,
			Error:      e.lastErr,
		}
		st.Entries = append(st.Entries, es)
	}
	return st
}

func (g *Group) acquireOne(e *entry) error {
	g.mu.RLock()
	held := e.held
	g.mu.RUnlock()
	if held {
		return nil
	}

	addr := fmt.Sprintf("%s/%d", e.cfg.IP, e.cfg.CIDR)

	// Check if already present (e.g. leftover from crash)
	if isIPOnInterface(e.ip, e.cfg.Interface) {
		g.logger.Info("VIP already present on interface, claiming",
			"vip", addr, "interface", e.cfg.Interface)
		g.mu.Lock()
		e.held = true
		e.acquireAt = time.Now()
		e.lastErr = ""
		g.mu.Unlock()
		sendGratuitousARP(e.cfg.IP, e.cfg.Interface, g.logger)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := runCmd(ctx, "ip", "addr", "add", addr, "dev", e.cfg.Interface)
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		// "RTNETLINK answers: File exists" means it's already there
		if strings.Contains(outStr, "File exists") {
			g.mu.Lock()
			e.held = true
			e.acquireAt = time.Now()
			e.lastErr = ""
			g.mu.Unlock()
			sendGratuitousARP(e.cfg.IP, e.cfg.Interface, g.logger)
			return nil
		}
		return fmt.Errorf("ip addr add %s dev %s: %w (%s)", addr, e.cfg.Interface, err, outStr)
	}

	g.mu.Lock()
	e.held = true
	e.acquireAt = time.Now()
	e.lastErr = ""
	g.mu.Unlock()

	g.logger.Warn("VIP acquired", "vip", addr, "interface", e.cfg.Interface)
	sendGratuitousARP(e.cfg.IP, e.cfg.Interface, g.logger)
	return nil
}

func (g *Group) releaseOne(e *entry) error {
	g.mu.RLock()
	held := e.held
	g.mu.RUnlock()
	if !held {
		return nil
	}

	addr := fmt.Sprintf("%s/%d", e.cfg.IP, e.cfg.CIDR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := runCmd(ctx, "ip", "addr", "del", addr, "dev", e.cfg.Interface)
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "Cannot assign") || strings.Contains(outStr, "ADDRNOTAVAIL") {
			g.mu.Lock()
			e.held = false
			g.mu.Unlock()
			return nil
		}
		return fmt.Errorf("ip addr del %s dev %s: %w (%s)", addr, e.cfg.Interface, err, outStr)
	}

	g.mu.Lock()
	e.held = false
	g.mu.Unlock()

	g.logger.Warn("VIP released", "vip", addr, "interface", e.cfg.Interface)
	return nil
}

// sendGratuitousARP sends a GARP for the given IP on the interface.
func sendGratuitousARP(ipStr, iface string, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if path, err := exec.LookPath("arping"); err == nil {
		out, err := runCmd(ctx, path, "-U", "-c", "3", "-I", iface, ipStr)
		if err != nil {
			logger.Debug("arping GARP failed (non-fatal)", "error", err, "output", strings.TrimSpace(string(out)))
		} else {
			logger.Info("gratuitous ARP sent", "vip", ipStr, "interface", iface)
			return
		}
	}

	// Fallback
	runCmd(ctx, "ip", "neigh", "change", ipStr, "dev", iface, "nud", "reachable")
}

// isIPOnInterface checks if an IP is assigned to a specific interface.
func isIPOnInterface(ip net.IP, ifaceName string) bool {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.Equal(ip) {
			return true
		}
	}
	return false
}
