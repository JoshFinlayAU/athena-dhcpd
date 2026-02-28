// Package vrrp detects keepalived/VRRP state for HA status reporting.
package vrrp

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
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

// Status holds the detected VRRP/keepalived status.
type Status struct {
	Detected     bool   `json:"detected"`
	Running      bool   `json:"running"`
	State        State  `json:"state"`
	InstanceName string `json:"instance_name,omitempty"`
	VIP          string `json:"vip,omitempty"`
	VIPOnLocal   bool   `json:"vip_on_local"`
	Interface    string `json:"interface,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	DataFile     string `json:"data_file,omitempty"`
}

// Detect checks for keepalived/VRRP presence and returns current status.
func Detect(cfg config.VRRPConfig) Status {
	s := Status{}

	if !cfg.Enabled {
		return s
	}

	s.Detected = true
	s.Interface = cfg.Interface
	s.VIP = cfg.VIP

	// Check if keepalived process is running
	s.Running = isKeepalivedRunning()

	// Try to parse VRRP state from keepalived data file
	dataFile := cfg.DataFile
	if dataFile == "" {
		dataFile = findDataFile()
	}
	if dataFile != "" {
		s.DataFile = dataFile
		instanceName := cfg.InstanceName
		if inst, state, priority, err := parseDataFile(dataFile, instanceName); err == nil {
			s.State = state
			s.Priority = priority
			if inst != "" {
				s.InstanceName = inst
			}
		}
	}

	// Check if VIP is present on a local interface
	if cfg.VIP != "" {
		s.VIPOnLocal = isIPLocal(cfg.VIP)
		// If we couldn't parse state from data file, infer from VIP presence
		if s.State == "" || s.State == StateUnknown {
			if s.VIPOnLocal {
				s.State = StateMaster
			} else if s.Running {
				s.State = StateBackup
			} else {
				s.State = StateStopped
			}
		}
	} else if s.State == "" {
		if s.Running {
			s.State = StateUnknown
		} else {
			s.State = StateStopped
		}
	}

	if cfg.InstanceName != "" && s.InstanceName == "" {
		s.InstanceName = cfg.InstanceName
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

// parseDataFile reads a keepalived data dump and extracts VRRP instance info.
// If instanceName is empty, returns the first instance found.
// Format example:
//
//	------< VRRP Topology >------
//	 VRRP Instance = VI_1
//	   State = MASTER
//	   ...
//	   Priority = 100
func parseDataFile(path, instanceName string) (name string, state State, priority int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", StateUnknown, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var inInstance bool
	var currentInstance string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "VRRP Instance =") {
			currentInstance = strings.TrimSpace(strings.TrimPrefix(line, "VRRP Instance ="))
			if instanceName == "" || currentInstance == instanceName {
				inInstance = true
				name = currentInstance
			} else {
				inInstance = false
			}
			continue
		}

		if !inInstance {
			continue
		}

		if strings.HasPrefix(line, "State =") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "State ="))
			switch strings.ToUpper(val) {
			case "MASTER":
				state = StateMaster
			case "BACKUP":
				state = StateBackup
			case "FAULT":
				state = StateFault
			default:
				state = StateUnknown
			}
		}

		if strings.HasPrefix(line, "Priority =") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Priority ="))
			fmt.Sscanf(val, "%d", &priority)
		}

		// If we found both state and priority, we're done
		if state != "" && state != StateUnknown && priority > 0 {
			return name, state, priority, nil
		}
	}

	if name != "" {
		return name, state, priority, nil
	}

	return "", StateUnknown, 0, fmt.Errorf("no matching VRRP instance found")
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
