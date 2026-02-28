// Package vrrp auto-detects keepalived/VRRP state for HA status reporting.
// No configuration is needed — the package discovers keepalived by checking
// for the running process and parsing its data dump file.
package vrrp

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// State represents the VRRP instance state.
type State string

const (
	StateMaster  State = "MASTER"
	StateBackup  State = "BACKUP"
	StateFault   State = "FAULT"
	StateUnknown State = "UNKNOWN"
	StateStopped State = "STOPPED"
)

// defaultDataPaths are the common locations for keepalived's data dump file.
var defaultDataPaths = []string{
	"/tmp/keepalived.data",
	"/var/run/keepalived/keepalived.data",
	"/var/lib/keepalived/keepalived.data",
}

// Instance holds the detected state of a single VRRP instance.
type Instance struct {
	Name       string   `json:"name"`
	State      State    `json:"state"`
	Interface  string   `json:"interface,omitempty"`
	VIPs       []string `json:"vips,omitempty"`
	VIPOnLocal bool     `json:"vip_on_local"`
	Priority   int      `json:"priority,omitempty"`
}

// Status holds the auto-detected VRRP/keepalived status.
type Status struct {
	Detected  bool       `json:"detected"`
	Running   bool       `json:"running"`
	Instances []Instance `json:"instances,omitempty"`
}

// Detect checks for keepalived/VRRP presence and returns current status.
// Requires no configuration — fully automatic.
func Detect() *Status {
	running := isKeepalivedRunning()
	if !running {
		return nil
	}

	s := &Status{
		Detected: true,
		Running:  true,
	}

	// Try to parse VRRP instances from keepalived data file
	if dataFile := findDataFile(); dataFile != "" {
		if instances, err := parseDataFile(dataFile); err == nil {
			// Check which VIPs are on local interfaces
			for i := range instances {
				for _, vip := range instances[i].VIPs {
					if isIPLocal(vip) {
						instances[i].VIPOnLocal = true
						break
					}
				}
			}
			s.Instances = instances
		}
	}

	return s
}

// isKeepalivedRunning checks if a keepalived process is running by scanning /proc.
func isKeepalivedRunning() bool {
	// Check PID files first (faster)
	pidPaths := []string{
		"/var/run/keepalived.pid",
		"/run/keepalived.pid",
		"/var/run/keepalived/keepalived.pid",
	}
	for _, p := range pidPaths {
		if data, err := os.ReadFile(p); err == nil {
			pid := strings.TrimSpace(string(data))
			if _, err := os.Stat(fmt.Sprintf("/proc/%s", pid)); err == nil {
				return true
			}
		}
	}

	// Fallback: scan /proc for keepalived
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only check numeric directory names (PIDs)
		if len(e.Name()) == 0 || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%s/comm", e.Name()))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "keepalived" {
			return true
		}
	}
	return false
}

// findDataFile returns the first existing keepalived data file path.
func findDataFile() string {
	for _, p := range defaultDataPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			// Only use if modified in the last hour (stale files are useless)
			if time.Since(info.ModTime()) < time.Hour {
				return p
			}
		}
	}
	return ""
}

// parseDataFile reads a keepalived data dump and extracts all VRRP instances.
// Format example:
//
//	------< VRRP Topology >------
//	 VRRP Instance = VI_1
//	   State = MASTER
//	   Interface = eth0
//	   Priority = 100
//	   Virtual IP = 1
//	     10.0.0.1/32 dev eth0
func parseDataFile(path string) ([]Instance, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var instances []Instance
	var cur *Instance
	var inVIPs bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// New instance block
		if strings.HasPrefix(line, "VRRP Instance =") {
			if cur != nil {
				instances = append(instances, *cur)
			}
			cur = &Instance{
				Name: strings.TrimSpace(strings.TrimPrefix(line, "VRRP Instance =")),
			}
			inVIPs = false
			continue
		}

		if cur == nil {
			continue
		}

		// Entering VIP list section
		if strings.HasPrefix(line, "Virtual IP =") {
			inVIPs = true
			continue
		}

		// VIP lines are indented IPs like "10.0.0.1/32 dev eth0" or just "10.0.0.1"
		if inVIPs {
			if line == "" || strings.HasPrefix(line, "------") || strings.Contains(line, "=") {
				inVIPs = false
				// fall through to parse this line normally
			} else {
				// Extract just the IP (strip /mask and "dev ..." suffix)
				ip := strings.Fields(line)[0]
				if idx := strings.Index(ip, "/"); idx != -1 {
					ip = ip[:idx]
				}
				if net.ParseIP(ip) != nil {
					cur.VIPs = append(cur.VIPs, ip)
				}
				continue
			}
		}

		if strings.HasPrefix(line, "State =") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "State ="))
			switch strings.ToUpper(val) {
			case "MASTER":
				cur.State = StateMaster
			case "BACKUP":
				cur.State = StateBackup
			case "FAULT":
				cur.State = StateFault
			default:
				cur.State = StateUnknown
			}
		}

		if strings.HasPrefix(line, "Interface =") {
			cur.Interface = strings.TrimSpace(strings.TrimPrefix(line, "Interface ="))
		}

		if strings.HasPrefix(line, "Priority =") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Priority ="))
			fmt.Sscanf(val, "%d", &cur.Priority)
		}
	}

	// Don't forget the last instance
	if cur != nil {
		instances = append(instances, *cur)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no VRRP instances found in %s", path)
	}
	return instances, nil
}

// isIPLocal checks if an IP address is assigned to any local interface.
func isIPLocal(ipStr string) bool {
	targetIP := net.ParseIP(ipStr)
	if targetIP == nil {
		return false
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.Equal(targetIP) {
				return true
			}
		}
	}
	return false
}
