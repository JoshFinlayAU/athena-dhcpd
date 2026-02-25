package dnsproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/miekg/dns"
)

// Server is the built-in DNS proxy with local zone support and upstream forwarding.
type Server struct {
	cfg    *config.DNSProxyConfig
	zone   *Zone
	cache  *Cache
	logger *slog.Logger

	udpServer *dns.Server
	tcpServer *dns.Server
	dohServer *http.Server

	forwarders    []string
	zoneOverrides map[string]config.DNSZoneOverride // lowercased zone -> override
	cacheTTL      time.Duration

	mu      sync.RWMutex
	started bool
}

// NewServer creates a new DNS proxy server from config.
func NewServer(cfg *config.DNSProxyConfig, logger *slog.Logger) *Server {
	cacheTTL, err := time.ParseDuration(cfg.CacheTTL)
	if err != nil {
		cacheTTL = 5 * time.Minute
	}

	s := &Server{
		cfg:           cfg,
		zone:          NewZone(cfg.Domain, uint32(cfg.TTL)),
		cache:         NewCache(cfg.CacheSize),
		logger:        logger,
		forwarders:    cfg.Forwarders,
		zoneOverrides: make(map[string]config.DNSZoneOverride),
		cacheTTL:      cacheTTL,
	}

	// Index zone overrides by lowercase zone name
	for _, zo := range cfg.ZoneOverrides {
		key := strings.ToLower(dns.Fqdn(zo.Zone))
		s.zoneOverrides[key] = zo
	}

	// Load static records
	for _, rec := range cfg.StaticRecords {
		ttl := uint32(cfg.TTL)
		if rec.TTL > 0 {
			ttl = uint32(rec.TTL)
		}
		rr, err := ParseStaticRecord(rec.Name, rec.Type, rec.Value, ttl)
		if err != nil {
			logger.Warn("skipping invalid static DNS record",
				"name", rec.Name, "type", rec.Type, "error", err)
			continue
		}
		s.zone.Add(rr)
		logger.Debug("loaded static DNS record", "name", rec.Name, "type", rec.Type, "value", rec.Value)
	}

	return s
}

// Zone returns the local zone for external lease registration.
func (s *Server) Zone() *Zone {
	return s.zone
}

// Cache returns the DNS cache.
func (s *Server) Cache() *Cache {
	return s.cache
}

// Start begins listening for DNS queries on configured addresses.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("DNS proxy already started")
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleQuery)

	// UDP listener
	s.udpServer = &dns.Server{
		Addr:    s.cfg.ListenUDP,
		Net:     "udp",
		Handler: mux,
	}

	// TCP listener on same port
	_, port, _ := net.SplitHostPort(s.cfg.ListenUDP)
	if port == "" {
		port = "53"
	}
	tcpAddr := net.JoinHostPort("", port)
	s.tcpServer = &dns.Server{
		Addr:    tcpAddr,
		Net:     "tcp",
		Handler: mux,
	}

	// Start UDP
	go func() {
		s.logger.Info("DNS proxy UDP listener starting", "addr", s.cfg.ListenUDP)
		if err := s.udpServer.ListenAndServe(); err != nil {
			s.logger.Error("DNS UDP listener error", "error", err)
		}
	}()

	// Start TCP
	go func() {
		s.logger.Info("DNS proxy TCP listener starting", "addr", tcpAddr)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			s.logger.Error("DNS TCP listener error", "error", err)
		}
	}()

	// Start DoH if configured
	if s.cfg.ListenDoH != "" {
		if err := s.startDoH(ctx); err != nil {
			s.logger.Error("DoH listener failed to start", "error", err)
			// Non-fatal — UDP/TCP still work
		}
	}

	s.started = true
	s.logger.Info("DNS proxy started",
		"udp", s.cfg.ListenUDP,
		"doh", s.cfg.ListenDoH,
		"domain", s.cfg.Domain,
		"forwarders", len(s.forwarders),
		"zone_overrides", len(s.zoneOverrides),
		"static_records", s.zone.Count(),
		"cache_size", s.cfg.CacheSize)

	return nil
}

// Stop shuts down all DNS listeners.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return
	}

	if s.udpServer != nil {
		s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		s.tcpServer.Shutdown()
	}
	if s.dohServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.dohServer.Shutdown(ctx)
	}

	s.started = false
	s.logger.Info("DNS proxy stopped")
}

// handleQuery is the main DNS query handler. Pipeline: local zone → cache → forward.
func (s *Server) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		dns.HandleFailed(w, r)
		return
	}

	q := r.Question[0]
	qname := strings.ToLower(q.Name)

	s.logger.Debug("DNS query",
		"name", qname,
		"type", dns.TypeToString[q.Qtype],
		"source", w.RemoteAddr())

	// 1. Check local zone
	if rrs := s.zone.Lookup(qname, q.Qtype); len(rrs) > 0 {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.Authoritative = true
		resp.Answer = rrs
		w.WriteMsg(resp)
		s.logger.Debug("DNS query answered from local zone",
			"name", qname, "answers", len(rrs))
		return
	}

	// 2. Check cache
	if cached := s.cache.Get(qname, q.Qtype, q.Qclass); cached != nil {
		cached.SetReply(r)
		w.WriteMsg(cached)
		s.logger.Debug("DNS query answered from cache", "name", qname)
		return
	}

	// 3. Forward upstream
	resp, err := s.forward(r)
	if err != nil {
		s.logger.Debug("DNS forward failed", "name", qname, "error", err)
		dns.HandleFailed(w, r)
		return
	}

	// Cache the response
	s.cache.Set(resp, s.cacheTTL)

	resp.SetReply(r)
	w.WriteMsg(resp)
}

// forward sends a query to the appropriate upstream server.
func (s *Server) forward(r *dns.Msg) (*dns.Msg, error) {
	if len(r.Question) == 0 {
		return nil, fmt.Errorf("no question in query")
	}

	qname := strings.ToLower(r.Question[0].Name)

	// Check zone overrides — find the most specific match
	override, found := s.findZoneOverride(qname)
	if found {
		return s.forwardToOverride(r, override)
	}

	// Forward to configured upstream servers
	if len(s.forwarders) == 0 {
		return nil, fmt.Errorf("no upstream forwarders configured")
	}

	return s.forwardToUpstream(r, s.forwarders)
}

// findZoneOverride returns the most specific zone override for a query name.
func (s *Server) findZoneOverride(qname string) (config.DNSZoneOverride, bool) {
	// Walk up the domain labels to find the most specific override
	labels := dns.SplitDomainName(qname)
	for i := 0; i < len(labels); i++ {
		candidate := strings.ToLower(dns.Fqdn(strings.Join(labels[i:], ".")))
		if zo, ok := s.zoneOverrides[candidate]; ok {
			return zo, true
		}
	}
	return config.DNSZoneOverride{}, false
}

// forwardToOverride sends a query to a zone override destination.
func (s *Server) forwardToOverride(r *dns.Msg, zo config.DNSZoneOverride) (*dns.Msg, error) {
	if zo.DoH && zo.DoHURL != "" {
		return s.forwardDoH(r, zo.DoHURL)
	}

	ns := zo.Nameserver
	if !strings.Contains(ns, ":") {
		ns = ns + ":53"
	}

	return s.forwardToUpstream(r, []string{ns})
}

// forwardToUpstream tries each upstream server in order until one succeeds.
func (s *Server) forwardToUpstream(r *dns.Msg, servers []string) (*dns.Msg, error) {
	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for _, server := range servers {
		addr := server
		if !strings.Contains(addr, ":") {
			addr = addr + ":53"
		}

		resp, _, err := client.Exchange(r, addr)
		if err != nil {
			lastErr = fmt.Errorf("forwarding to %s: %w", addr, err)
			s.logger.Debug("upstream DNS server failed", "server", addr, "error", err)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all upstream servers failed: %w", lastErr)
}

// forwardDoH sends a DNS query via DNS-over-HTTPS (RFC 8484).
func (s *Server) forwardDoH(r *dns.Msg, url string) (*dns.Msg, error) {
	packed, err := r.Pack()
	if err != nil {
		return nil, fmt.Errorf("packing DNS query for DoH: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(packed))
	if err != nil {
		return nil, fmt.Errorf("creating DoH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 65535))
	if err != nil {
		return nil, fmt.Errorf("reading DoH response: %w", err)
	}

	dnsResp := new(dns.Msg)
	if err := dnsResp.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpacking DoH response: %w", err)
	}

	return dnsResp, nil
}

// startDoH starts the DNS-over-HTTPS listener.
func (s *Server) startDoH(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/dns-query", s.handleDoH)

	s.dohServer = &http.Server{
		Addr:         s.cfg.ListenDoH,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Check if TLS is configured
	hasTLS := s.cfg.DoHTLS.CertFile != "" && s.cfg.DoHTLS.KeyFile != ""

	go func() {
		var err error
		if hasTLS {
			s.logger.Info("DNS proxy DoH (HTTPS) listener starting", "addr", s.cfg.ListenDoH)
			err = s.dohServer.ListenAndServeTLS(s.cfg.DoHTLS.CertFile, s.cfg.DoHTLS.KeyFile)
		} else {
			s.logger.Info("DNS proxy DoH (HTTP) listener starting", "addr", s.cfg.ListenDoH)
			err = s.dohServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("DoH listener error", "error", err)
		}
	}()

	return nil
}

// handleDoH handles DNS-over-HTTPS requests (RFC 8484).
func (s *Server) handleDoH(w http.ResponseWriter, r *http.Request) {
	var wireMsg []byte
	var err error

	switch r.Method {
	case http.MethodPost:
		if ct := r.Header.Get("Content-Type"); ct != "application/dns-message" {
			http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
			return
		}
		wireMsg, err = io.ReadAll(io.LimitReader(r.Body, 65535))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

	case http.MethodGet:
		// RFC 8484 §4.1 — GET with ?dns= base64url parameter
		dnsParam := r.URL.Query().Get("dns")
		if dnsParam == "" {
			http.Error(w, "missing dns parameter", http.StatusBadRequest)
			return
		}
		wireMsg, err = decodeBase64URL(dnsParam)
		if err != nil {
			http.Error(w, "invalid dns parameter", http.StatusBadRequest)
			return
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse DNS message
	msg := new(dns.Msg)
	if err := msg.Unpack(wireMsg); err != nil {
		http.Error(w, "invalid DNS message", http.StatusBadRequest)
		return
	}

	// Process through our handler using a DoH response writer
	dohWriter := &dohResponseWriter{}
	s.handleQuery(dohWriter, msg)

	if dohWriter.msg == nil {
		http.Error(w, "no response", http.StatusInternalServerError)
		return
	}

	packed, err := dohWriter.msg.Pack()
	if err != nil {
		http.Error(w, "failed to pack response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/dns-message")
	w.WriteHeader(http.StatusOK)
	w.Write(packed)
}

// dohResponseWriter implements dns.ResponseWriter for DoH processing.
type dohResponseWriter struct {
	msg *dns.Msg
}

func (d *dohResponseWriter) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (d *dohResponseWriter) RemoteAddr() net.Addr { return &net.TCPAddr{} }
func (d *dohResponseWriter) WriteMsg(msg *dns.Msg) error {
	d.msg = msg
	return nil
}
func (d *dohResponseWriter) Write(b []byte) (int, error) {
	msg := new(dns.Msg)
	if err := msg.Unpack(b); err != nil {
		return 0, err
	}
	d.msg = msg
	return len(b), nil
}
func (d *dohResponseWriter) Close() error        { return nil }
func (d *dohResponseWriter) TsigStatus() error   { return nil }
func (d *dohResponseWriter) TsigTimersOnly(bool) {}
func (d *dohResponseWriter) Hijack()             {}

// decodeBase64URL decodes a base64url-encoded string (RFC 4648 §5, no padding).
func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// RegisterLease adds A and PTR records for a DHCP lease to the local zone.
func (s *Server) RegisterLease(hostname string, ip net.IP) {
	if !s.cfg.RegisterLeases || hostname == "" || ip == nil {
		return
	}
	s.zone.RegisterLease(hostname, ip, s.cfg.ForwardLeasesPTR)
	s.logger.Debug("DNS proxy registered lease",
		"hostname", hostname, "ip", ip.String())
}

// UnregisterLease removes A and PTR records for a DHCP lease.
func (s *Server) UnregisterLease(hostname string, ip net.IP) {
	if !s.cfg.RegisterLeases {
		return
	}
	s.zone.UnregisterLease(hostname, ip)
	s.logger.Debug("DNS proxy unregistered lease",
		"hostname", hostname, "ip", ip)
}

// SubscribeToEvents starts a goroutine that listens for lease events and
// registers/unregisters DNS records accordingly. Cancel the context to stop.
func (s *Server) SubscribeToEvents(ctx context.Context, eventCh <-chan events.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				s.handleEvent(evt)
			}
		}
	}()
}

func (s *Server) handleEvent(evt events.Event) {
	if evt.Lease == nil {
		return
	}

	hostname := evt.Lease.Hostname
	ip := evt.Lease.IP

	switch evt.Type {
	case events.EventLeaseAck, events.EventLeaseRenew:
		s.RegisterLease(hostname, ip)
	case events.EventLeaseRelease, events.EventLeaseExpire:
		s.UnregisterLease(hostname, ip)
	}
}

// FlushCache clears the DNS response cache.
func (s *Server) FlushCache() {
	s.cache.Flush()
	s.logger.Info("DNS proxy cache flushed")
}

// Stats returns basic DNS proxy statistics.
func (s *Server) Stats() map[string]interface{} {
	return map[string]interface{}{
		"zone_records":  s.zone.Count(),
		"cache_entries": s.cache.Size(),
		"forwarders":    len(s.forwarders),
		"overrides":     len(s.zoneOverrides),
		"domain":        s.cfg.Domain,
	}
}
