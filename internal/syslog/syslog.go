// Package syslog provides SIEM event forwarding for athena-dhcpd.
// It subscribes to the event bus and forwards events in multiple formats
// (RFC 5424 syslog, CEF, JSON) to multiple outputs (remote syslog, HTTP/HEC, file).
package syslog

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
)

// Facility values (RFC 5424)
const (
	FacilityDaemon = 3
	FacilityLocal0 = 16
	FacilityLocal7 = 23
)

// Severity values (RFC 5424)
const (
	SeverityEmergency = iota
	SeverityAlert
	SeverityCritical
	SeverityError
	SeverityWarning
	SeverityNotice
	SeverityInfo
	SeverityDebug
)

// Format constants
const (
	FormatRFC5424 = "rfc5424"
	FormatCEF     = "cef"
	FormatJSON    = "json"
)

// Forwarder subscribes to the event bus and forwards events to configured outputs.
type Forwarder struct {
	cfg    config.SyslogConfig
	bus    *events.Bus
	logger *slog.Logger
	ch     chan events.Event
	done   chan struct{}

	// Syslog output
	syslogMu   sync.Mutex
	syslogConn net.Conn

	// HTTP output
	httpClient *http.Client

	// File output
	fileMu     sync.Mutex
	fileHandle *os.File
	fileSize   int64
	hostname   string
}

// NewForwarder creates a new SIEM event forwarder.
func NewForwarder(cfg config.SyslogConfig, bus *events.Bus, logger *slog.Logger) *Forwarder {
	if cfg.Tag == "" {
		cfg.Tag = "athena-dhcpd"
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "udp"
	}
	if cfg.Facility == 0 {
		cfg.Facility = FacilityLocal0
	}
	if cfg.Format == "" {
		cfg.Format = FormatRFC5424
	}
	if cfg.CEFDeviceVendor == "" {
		cfg.CEFDeviceVendor = "athena-dhcpd"
	}
	if cfg.CEFDeviceProduct == "" {
		cfg.CEFDeviceProduct = "DHCP Server"
	}
	if cfg.CEFDeviceVersion == "" {
		cfg.CEFDeviceVersion = "1.0"
	}
	if cfg.FileMaxSizeMB == 0 {
		cfg.FileMaxSizeMB = 100
	}
	if cfg.FileMaxBackups == 0 {
		cfg.FileMaxBackups = 5
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "-"
	}

	return &Forwarder{
		cfg:      cfg,
		bus:      bus,
		logger:   logger,
		done:     make(chan struct{}),
		hostname: hostname,
	}
}

// Start subscribes to the event bus and begins forwarding to all enabled outputs.
func (f *Forwarder) Start() error {
	started := 0

	// Start syslog output
	if f.cfg.Address != "" {
		conn, err := net.DialTimeout(f.cfg.Protocol, f.cfg.Address, 5*time.Second)
		if err != nil {
			return fmt.Errorf("connecting to syslog %s://%s: %w", f.cfg.Protocol, f.cfg.Address, err)
		}
		f.syslogMu.Lock()
		f.syslogConn = conn
		f.syslogMu.Unlock()
		f.logger.Info("syslog output started", "address", f.cfg.Address, "protocol", f.cfg.Protocol)
		started++
	}

	// Start HTTP output
	if f.cfg.HTTPEnabled && f.cfg.HTTPEndpoint != "" {
		timeout := 5 * time.Second
		if f.cfg.HTTPTimeout != "" {
			if d, err := time.ParseDuration(f.cfg.HTTPTimeout); err == nil {
				timeout = d
			}
		}
		transport := &http.Transport{}
		if f.cfg.HTTPInsecure {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		}
		f.httpClient = &http.Client{Timeout: timeout, Transport: transport}
		f.logger.Info("HTTP output started", "endpoint", f.cfg.HTTPEndpoint)
		started++
	}

	// Start file output
	if f.cfg.FileEnabled && f.cfg.FilePath != "" {
		dir := filepath.Dir(f.cfg.FilePath)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("creating log directory %s: %w", dir, err)
		}
		fh, err := os.OpenFile(f.cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
		if err != nil {
			return fmt.Errorf("opening log file %s: %w", f.cfg.FilePath, err)
		}
		info, _ := fh.Stat()
		f.fileMu.Lock()
		f.fileHandle = fh
		if info != nil {
			f.fileSize = info.Size()
		}
		f.fileMu.Unlock()
		f.logger.Info("file output started", "path", f.cfg.FilePath)
		started++
	}

	if started == 0 {
		return fmt.Errorf("no outputs configured (enable syslog address, HTTP endpoint, or file path)")
	}

	f.ch = f.bus.Subscribe(500)
	go f.loop()

	f.logger.Info("SIEM forwarder started", "format", f.cfg.Format, "outputs", started)
	return nil
}

// Stop shuts down the forwarder and all outputs.
func (f *Forwarder) Stop() {
	close(f.done)
	if f.ch != nil {
		f.bus.Unsubscribe(f.ch)
	}

	f.syslogMu.Lock()
	if f.syslogConn != nil {
		f.syslogConn.Close()
	}
	f.syslogMu.Unlock()

	f.fileMu.Lock()
	if f.fileHandle != nil {
		f.fileHandle.Close()
	}
	f.fileMu.Unlock()

	f.logger.Info("SIEM forwarder stopped")
}

func (f *Forwarder) loop() {
	for {
		select {
		case evt, ok := <-f.ch:
			if !ok {
				return
			}
			f.forward(evt)
		case <-f.done:
			return
		}
	}
}

func (f *Forwarder) forward(evt events.Event) {
	formatted := f.formatEvent(evt)

	// Send to syslog
	if f.syslogConn != nil {
		f.sendSyslog(evt, formatted)
	}

	// Send to HTTP
	if f.httpClient != nil {
		f.sendHTTP(evt, formatted)
	}

	// Write to file
	if f.fileHandle != nil {
		f.writeFile(formatted)
	}
}

// formatEvent returns the formatted event string based on the configured format.
func (f *Forwarder) formatEvent(evt events.Event) string {
	switch f.cfg.Format {
	case FormatCEF:
		return f.formatCEF(evt)
	case FormatJSON:
		return f.formatJSON(evt)
	default:
		return formatKV(evt)
	}
}

// --- Syslog output ---

func (f *Forwarder) sendSyslog(evt events.Event, msg string) {
	severity := eventSeverity(evt.Type)
	priority := f.cfg.Facility*8 + severity

	ts := evt.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z")

	var line string
	if f.cfg.Format == FormatCEF {
		// CEF is self-describing, wrap in syslog envelope
		line = fmt.Sprintf("<%d>1 %s %s %s - - - %s\n", priority, ts, f.hostname, f.cfg.Tag, msg)
	} else {
		line = fmt.Sprintf("<%d>1 %s %s %s - - - %s\n", priority, ts, f.hostname, f.cfg.Tag, msg)
	}

	f.syslogMu.Lock()
	defer f.syslogMu.Unlock()

	if f.syslogConn == nil {
		return
	}

	if _, err := f.syslogConn.Write([]byte(line)); err != nil {
		f.logger.Debug("syslog write failed, reconnecting", "error", err)
		f.syslogConn.Close()
		conn, err := net.DialTimeout(f.cfg.Protocol, f.cfg.Address, 3*time.Second)
		if err != nil {
			f.logger.Warn("syslog reconnect failed", "error", err)
			f.syslogConn = nil
			return
		}
		f.syslogConn = conn
		f.syslogConn.Write([]byte(line))
	}
}

// --- HTTP output (Splunk HEC, Elasticsearch, generic) ---

func (f *Forwarder) sendHTTP(evt events.Event, formatted string) {
	var body []byte

	// Detect Splunk HEC by endpoint path
	if strings.Contains(f.cfg.HTTPEndpoint, "/services/collector") {
		// Splunk HEC expects {"event": <data>, "sourcetype": "...", "time": <epoch>}
		wrapper := map[string]interface{}{
			"time":       evt.Timestamp.Unix(),
			"sourcetype": "athena:dhcp",
			"source":     f.cfg.Tag,
			"host":       f.hostname,
		}
		if f.cfg.Format == FormatJSON {
			// Embed the full JSON event object
			var evtData interface{}
			json.Unmarshal([]byte(formatted), &evtData)
			wrapper["event"] = evtData
		} else {
			wrapper["event"] = formatted
		}
		body, _ = json.Marshal(wrapper)
	} else {
		// Generic HTTP POST — send formatted message as-is
		if f.cfg.Format == FormatJSON {
			body = []byte(formatted)
		} else {
			body, _ = json.Marshal(map[string]string{"message": formatted, "timestamp": evt.Timestamp.UTC().Format(time.RFC3339Nano)})
		}
	}

	req, err := http.NewRequest("POST", f.cfg.HTTPEndpoint, bytes.NewReader(body))
	if err != nil {
		f.logger.Debug("failed to create HTTP request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Set auth token — Splunk uses "Splunk <token>", others use "Bearer <token>"
	if f.cfg.HTTPToken != "" {
		if strings.Contains(f.cfg.HTTPEndpoint, "/services/collector") {
			req.Header.Set("Authorization", "Splunk "+f.cfg.HTTPToken)
		} else {
			req.Header.Set("Authorization", "Bearer "+f.cfg.HTTPToken)
		}
	}

	// Custom headers
	for k, v := range f.cfg.HTTPHeaders {
		req.Header.Set(k, v)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		f.logger.Debug("HTTP output send failed", "error", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		f.logger.Debug("HTTP output returned error", "status", resp.StatusCode)
	}
}

// --- File output with rotation ---

func (f *Forwarder) writeFile(msg string) {
	line := msg + "\n"

	f.fileMu.Lock()
	defer f.fileMu.Unlock()

	if f.fileHandle == nil {
		return
	}

	n, err := f.fileHandle.WriteString(line)
	if err != nil {
		f.logger.Debug("file write failed", "error", err)
		return
	}
	f.fileSize += int64(n)

	// Check if rotation needed
	maxBytes := int64(f.cfg.FileMaxSizeMB) * 1024 * 1024
	if maxBytes > 0 && f.fileSize >= maxBytes {
		f.rotateFile()
	}
}

func (f *Forwarder) rotateFile() {
	f.fileHandle.Close()

	// Compress current file to .gz and shift backups
	for i := f.cfg.FileMaxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d.gz", f.cfg.FilePath, i-1)
		dst := fmt.Sprintf("%s.%d.gz", f.cfg.FilePath, i)
		if i == 1 {
			src = f.cfg.FilePath
			// Compress the current log file
			f.compressFile(src, dst)
			continue
		}
		os.Rename(src, dst)
	}

	// Remove excess backups
	excess := fmt.Sprintf("%s.%d.gz", f.cfg.FilePath, f.cfg.FileMaxBackups+1)
	os.Remove(excess)

	// Truncate and reopen
	fh, err := os.OpenFile(f.cfg.FilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		f.logger.Warn("failed to reopen log file after rotation", "error", err)
		f.fileHandle = nil
		return
	}
	f.fileHandle = fh
	f.fileSize = 0
}

func (f *Forwarder) compressFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	io.Copy(gz, in)
	gz.Close()
}

// --- Formatters ---

// FormatMessage formats an event into a key=value string (exported for testing).
func FormatMessage(evt events.Event) string {
	return formatKV(evt)
}

// FormatCEFMessage formats an event into CEF format (exported for testing).
func FormatCEFMessage(evt events.Event) string {
	f := &Forwarder{cfg: config.SyslogConfig{
		CEFDeviceVendor:  "athena-dhcpd",
		CEFDeviceProduct: "DHCP Server",
		CEFDeviceVersion: "1.0",
	}}
	return f.formatCEF(evt)
}

func formatKV(evt events.Event) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("event=%s", evt.Type))

	if evt.Lease != nil {
		l := evt.Lease
		if l.IP != nil {
			parts = append(parts, fmt.Sprintf("ip=%s", l.IP))
		}
		if l.MAC != "" {
			parts = append(parts, fmt.Sprintf("mac=%s", l.MAC))
		}
		if l.Hostname != "" {
			parts = append(parts, fmt.Sprintf("hostname=%s", l.Hostname))
		}
		if l.Subnet != "" {
			parts = append(parts, fmt.Sprintf("subnet=%s", l.Subnet))
		}
		if l.ClientID != "" {
			parts = append(parts, fmt.Sprintf("client_id=%s", l.ClientID))
		}
	}

	if evt.Conflict != nil {
		c := evt.Conflict
		if c.IP != nil {
			parts = append(parts, fmt.Sprintf("conflict_ip=%s", c.IP))
		}
		parts = append(parts, fmt.Sprintf("method=%s", c.DetectionMethod))
	}

	if evt.Rogue != nil {
		r := evt.Rogue
		if r.ServerIP != nil {
			parts = append(parts, fmt.Sprintf("rogue_server=%s", r.ServerIP))
		}
		parts = append(parts, fmt.Sprintf("count=%d", r.Count))
	}

	if evt.Reason != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", evt.Reason))
	}

	return strings.Join(parts, " ")
}

// formatCEF produces ArcSight Common Event Format messages.
// CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
func (f *Forwarder) formatCEF(evt events.Event) string {
	sigID := cefSignatureID(evt.Type)
	name := cefEventName(evt.Type)
	severity := cefSeverity(evt.Type)

	var ext []string
	ext = append(ext, fmt.Sprintf("rt=%d", evt.Timestamp.UnixMilli()))

	if evt.Lease != nil {
		l := evt.Lease
		if l.IP != nil {
			ext = append(ext, fmt.Sprintf("dst=%s", l.IP))
		}
		if l.MAC != "" {
			ext = append(ext, fmt.Sprintf("dmac=%s", l.MAC))
		}
		if l.Hostname != "" {
			ext = append(ext, fmt.Sprintf("dhost=%s", cefEscape(l.Hostname)))
		}
		if l.Subnet != "" {
			ext = append(ext, fmt.Sprintf("cs1=%s cs1Label=Subnet", cefEscape(l.Subnet)))
		}
		if l.ClientID != "" {
			ext = append(ext, fmt.Sprintf("cs2=%s cs2Label=ClientID", cefEscape(l.ClientID)))
		}
		if l.Pool != "" {
			ext = append(ext, fmt.Sprintf("cs3=%s cs3Label=Pool", cefEscape(l.Pool)))
		}
		if l.FQDN != "" {
			ext = append(ext, fmt.Sprintf("cs4=%s cs4Label=FQDN", cefEscape(l.FQDN)))
		}
		if l.Start != 0 {
			ext = append(ext, fmt.Sprintf("cn1=%d cn1Label=LeaseStart", l.Start))
		}
		if l.Expiry != 0 {
			ext = append(ext, fmt.Sprintf("cn2=%d cn2Label=LeaseExpiry", l.Expiry))
		}
		if l.Relay != nil {
			if l.Relay.GIAddr != nil {
				ext = append(ext, fmt.Sprintf("cs5=%s cs5Label=RelayGateway", l.Relay.GIAddr))
			}
			if l.Relay.CircuitID != "" {
				ext = append(ext, fmt.Sprintf("cs6=%s cs6Label=RelayCircuitID", cefEscape(l.Relay.CircuitID)))
			}
		}
	}

	if evt.Conflict != nil {
		c := evt.Conflict
		if c.IP != nil {
			ext = append(ext, fmt.Sprintf("dst=%s", c.IP))
		}
		ext = append(ext, fmt.Sprintf("cs1=%s cs1Label=DetectionMethod", cefEscape(c.DetectionMethod)))
		if c.ResponderMAC != "" {
			ext = append(ext, fmt.Sprintf("smac=%s", c.ResponderMAC))
		}
		if c.Subnet != "" {
			ext = append(ext, fmt.Sprintf("cs2=%s cs2Label=Subnet", cefEscape(c.Subnet)))
		}
	}

	if evt.Rogue != nil {
		r := evt.Rogue
		if r.ServerIP != nil {
			ext = append(ext, fmt.Sprintf("src=%s", r.ServerIP))
		}
		if r.ServerMAC != "" {
			ext = append(ext, fmt.Sprintf("smac=%s", r.ServerMAC))
		}
		ext = append(ext, fmt.Sprintf("cn1=%d cn1Label=DetectionCount", r.Count))
	}

	if evt.Reason != "" {
		ext = append(ext, fmt.Sprintf("msg=%s", cefEscape(evt.Reason)))
	}

	return fmt.Sprintf("CEF:0|%s|%s|%s|%s|%s|%d|%s",
		cefEscape(f.cfg.CEFDeviceVendor),
		cefEscape(f.cfg.CEFDeviceProduct),
		cefEscape(f.cfg.CEFDeviceVersion),
		sigID,
		cefEscape(name),
		severity,
		strings.Join(ext, " "),
	)
}

func (f *Forwarder) formatJSON(evt events.Event) string {
	data, _ := json.Marshal(evt)
	return string(data)
}

// --- CEF helpers ---

func cefEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `|`, `\|`)
	s = strings.ReplaceAll(s, `=`, `\=`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func cefSignatureID(t events.EventType) string {
	switch t {
	case events.EventLeaseDiscover:
		return "100"
	case events.EventLeaseOffer:
		return "101"
	case events.EventLeaseAck:
		return "102"
	case events.EventLeaseRenew:
		return "103"
	case events.EventLeaseNak:
		return "104"
	case events.EventLeaseRelease:
		return "105"
	case events.EventLeaseDecline:
		return "106"
	case events.EventLeaseExpire:
		return "107"
	case events.EventConflictDetected:
		return "200"
	case events.EventConflictDecline:
		return "201"
	case events.EventConflictResolved:
		return "202"
	case events.EventConflictPermanent:
		return "203"
	case events.EventHAFailover:
		return "300"
	case events.EventHAPeerUp:
		return "301"
	case events.EventHAPeerDown:
		return "302"
	case events.EventHASyncComplete:
		return "303"
	case events.EventRogueDetected:
		return "400"
	case events.EventRogueResolved:
		return "401"
	case events.EventAnomalyDetected:
		return "500"
	default:
		return "999"
	}
}

func cefEventName(t events.EventType) string {
	switch t {
	case events.EventLeaseDiscover:
		return "DHCP Discover"
	case events.EventLeaseOffer:
		return "DHCP Offer"
	case events.EventLeaseAck:
		return "DHCP Lease Granted"
	case events.EventLeaseRenew:
		return "DHCP Lease Renewed"
	case events.EventLeaseNak:
		return "DHCP NAK"
	case events.EventLeaseRelease:
		return "DHCP Lease Released"
	case events.EventLeaseDecline:
		return "DHCP Lease Declined"
	case events.EventLeaseExpire:
		return "DHCP Lease Expired"
	case events.EventConflictDetected:
		return "IP Conflict Detected"
	case events.EventConflictDecline:
		return "IP Conflict Client Decline"
	case events.EventConflictResolved:
		return "IP Conflict Resolved"
	case events.EventConflictPermanent:
		return "IP Conflict Permanent"
	case events.EventHAFailover:
		return "HA Failover"
	case events.EventHAPeerUp:
		return "HA Peer Up"
	case events.EventHAPeerDown:
		return "HA Peer Down"
	case events.EventHASyncComplete:
		return "HA Sync Complete"
	case events.EventRogueDetected:
		return "Rogue DHCP Server Detected"
	case events.EventRogueResolved:
		return "Rogue DHCP Server Resolved"
	case events.EventAnomalyDetected:
		return "Network Anomaly Detected"
	default:
		return string(t)
	}
}

// cefSeverity maps event types to CEF severity (0-10 scale).
func cefSeverity(t events.EventType) int {
	switch t {
	case events.EventRogueDetected:
		return 7
	case events.EventConflictPermanent:
		return 7
	case events.EventConflictDetected:
		return 5
	case events.EventAnomalyDetected:
		return 5
	case events.EventHAFailover, events.EventHAPeerDown:
		return 5
	case events.EventLeaseDecline:
		return 4
	case events.EventConflictDecline:
		return 4
	case events.EventLeaseNak:
		return 3
	case events.EventConflictResolved, events.EventRogueResolved:
		return 2
	case events.EventHAPeerUp, events.EventHASyncComplete:
		return 1
	default:
		return 1
	}
}

func eventSeverity(t events.EventType) int {
	switch t {
	case events.EventRogueDetected:
		return SeverityWarning
	case events.EventConflictDetected, events.EventConflictPermanent:
		return SeverityWarning
	case events.EventAnomalyDetected:
		return SeverityWarning
	case events.EventLeaseDecline:
		return SeverityNotice
	case events.EventHAFailover:
		return SeverityNotice
	default:
		return SeverityInfo
	}
}
