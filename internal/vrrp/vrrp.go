// Package vrrp auto-detects keepalived/VRRP state for HA status reporting.
// No configuration is needed — the package discovers keepalived by checking
// for the running process, signalling it to dump fresh data, and parsing
// both the data and stats files.
package vrrp

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// State represents the VRRP instance state.
type State string

const (
	StateMaster  State = "MASTER"
	StateBackup  State = "BACKUP"
	StateFault   State = "FAULT"
	StateUnknown State = "UNKNOWN"
)

// PID file locations (checked in order).
var pidPaths = []string{
	"/var/run/keepalived.pid",
	"/run/keepalived.pid",
	"/var/run/keepalived/keepalived.pid",
}

// Data/stats file locations (checked in order).
var dataFilePaths = []string{
	"/tmp/keepalived.data",
	"/var/run/keepalived/keepalived.data",
	"/var/lib/keepalived/keepalived.data",
}

var statsFilePaths = []string{
	"/tmp/keepalived.stats",
	"/var/run/keepalived/keepalived.stats",
	"/var/lib/keepalived/keepalived.stats",
}

// InstanceStats holds advertisement and state-change counters from keepalived.stats.
type InstanceStats struct {
	AdvertisementsRx int `json:"advertisements_rx"`
	AdvertisementsTx int `json:"advertisements_tx"`
	BecameMaster     int `json:"became_master"`
	ReleasedMaster   int `json:"released_master"`
	PacketErrors     int `json:"packet_errors,omitempty"`
	AuthErrors       int `json:"auth_errors,omitempty"`
}

// Instance holds the detected state of a single VRRP instance.
type Instance struct {
	Name       string         `json:"name"`
	State      State          `json:"state"`
	Interface  string         `json:"interface,omitempty"`
	VIPs       []string       `json:"vips,omitempty"`
	VIPOnLocal bool           `json:"vip_on_local"`
	Priority   int            `json:"priority,omitempty"`
	Stats      *InstanceStats `json:"stats,omitempty"`
}

// Status holds the auto-detected VRRP/keepalived status.
type Status struct {
	Detected  bool       `json:"detected"`
	Running   bool       `json:"running"`
	PID       int        `json:"pid,omitempty"`
	Instances []Instance `json:"instances,omitempty"`
}

// Detect checks for keepalived/VRRP presence and returns current status.
// Sends SIGUSR1 to keepalived to trigger a fresh data dump, then parses
// both the data file (state, VIPs) and stats file (counters).
func Detect() *Status {
	pid := findKeepalivedPID()
	if pid == 0 {
		return nil
	}

	s := &Status{
		Detected: true,
		Running:  true,
		PID:      pid,
	}

	// Signal keepalived to dump fresh data + stats
	signalKeepalived(pid)

	// Parse data file for instance state, VIPs, priority
	if dataFile := findFile(dataFilePaths); dataFile != "" {
		if instances, err := parseDataFile(dataFile); err == nil {
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

	// Parse stats file and merge into instances
	if statsFile := findFile(statsFilePaths); statsFile != "" {
		if statsMap, err := parseStatsFile(statsFile); err == nil {
			for i := range s.Instances {
				if st, ok := statsMap[s.Instances[i].Name]; ok {
					s.Instances[i].Stats = &st
				}
			}
		}
	}

	return s
}

// findKeepalivedPID returns the keepalived PID, or 0 if not running.
func findKeepalivedPID() int {
	// Check PID files first
	for _, p := range pidPaths {
		if data, err := os.ReadFile(p); err == nil {
			pidStr := strings.TrimSpace(string(data))
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
				return pid
			}
		}
	}

	// Fallback: scan /proc for keepalived
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if len(e.Name()) == 0 || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%s/comm", e.Name()))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "keepalived" {
			pid, _ := strconv.Atoi(e.Name())
			return pid
		}
	}
	return 0
}

// signalKeepalived sends SIGUSR1 to dump data and SIGUSR2 to dump stats.
// Waits briefly for the files to be written.
func signalKeepalived(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	// SIGUSR1 triggers data dump, SIGUSR2 triggers stats dump
	proc.Signal(syscall.SIGUSR1)
	proc.Signal(syscall.SIGUSR2)

	// Brief wait for keepalived to write the files
	time.Sleep(50 * time.Millisecond)
}

// findFile returns the first existing file from the candidate paths.
func findFile(paths []string) string {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// parseDataFile reads a keepalived data dump and extracts all VRRP instances.
// Format:
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

		if strings.HasPrefix(line, "Virtual IP =") {
			inVIPs = true
			continue
		}

		if inVIPs {
			if line == "" || strings.HasPrefix(line, "------") || strings.Contains(line, "=") {
				inVIPs = false
			} else {
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

	if cur != nil {
		instances = append(instances, *cur)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no VRRP instances found in %s", path)
	}
	return instances, nil
}

// parseStatsFile reads a keepalived stats dump.
// Format:
//
//	VRRP Instance: ATHENA_DNS
//	  Advertisements:
//	    Received: 4
//	    Sent: 135757
//	  Became master: 1
//	  Released master: 0
//	  Packet Errors:
//	    Length: 0
//	    TTL: 0
//	    Invalid Type: 0
//	    Advertisement Interval: 0
//	    Address List: 0
//	  Authentication Errors:
//	    Invalid Type: 0
//	    Type Mismatch: 0
//	    Failure: 0
func parseStatsFile(path string) (map[string]InstanceStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]InstanceStats)
	var curName string
	var cur InstanceStats
	var inSection string // "advertisements", "packet_errors", "auth_errors"

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// New instance block
		if strings.HasPrefix(trimmed, "VRRP Instance:") {
			if curName != "" {
				result[curName] = cur
			}
			curName = strings.TrimSpace(strings.TrimPrefix(trimmed, "VRRP Instance:"))
			cur = InstanceStats{}
			inSection = ""
			continue
		}

		if curName == "" {
			continue
		}

		// Section headers
		if trimmed == "Advertisements:" {
			inSection = "advertisements"
			continue
		}
		if trimmed == "Packet Errors:" {
			inSection = "packet_errors"
			continue
		}
		if trimmed == "Authentication Errors:" {
			inSection = "auth_errors"
			continue
		}
		if trimmed == "Priority Zero:" {
			inSection = "priority_zero"
			continue
		}

		// Key: Value lines
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := 0
			if len(parts) == 2 {
				val, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}

			switch inSection {
			case "advertisements":
				switch key {
				case "Received":
					cur.AdvertisementsRx = val
				case "Sent":
					cur.AdvertisementsTx = val
				}
			case "packet_errors":
				cur.PacketErrors += val
			case "auth_errors":
				cur.AuthErrors += val
			case "":
				switch key {
				case "Became master":
					cur.BecameMaster = val
				case "Released master":
					cur.ReleasedMaster = val
				}
			}
		}
	}

	if curName != "" {
		result[curName] = cur
	}

	return result, nil
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
