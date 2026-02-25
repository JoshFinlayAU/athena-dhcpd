package dnsproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/miekg/dns"
)

// ListStatus holds runtime state for a single DNS filter list.
type ListStatus struct {
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	Type            string    `json:"type"`
	Format          string    `json:"format"`
	Action          string    `json:"action"`
	Enabled         bool      `json:"enabled"`
	DomainCount     int       `json:"domain_count"`
	LastRefresh     time.Time `json:"last_refresh"`
	LastError       string    `json:"last_error,omitempty"`
	RefreshInterval string    `json:"refresh_interval"`
	NextRefresh     time.Time `json:"next_refresh"`
}

// ListManager manages dynamic DNS filter lists (blocklists and allowlists).
// Domains are stored in a map for O(1) lookup. Each list is downloaded from a
// URL and periodically refreshed on a configurable interval.
type ListManager struct {
	mu     sync.RWMutex
	lists  []managedList
	logger *slog.Logger
	client *http.Client
	cancel context.CancelFunc
}

type managedList struct {
	cfg     config.DNSListConfig
	domains map[string]struct{} // lowercased FQDN -> present
	status  ListStatus
}

// NewListManager creates a list manager from config. Call Start() to begin refresh loops.
func NewListManager(cfgs []config.DNSListConfig, logger *slog.Logger) *ListManager {
	lm := &ListManager{
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}

	for _, c := range cfgs {
		ml := managedList{
			cfg:     c,
			domains: make(map[string]struct{}),
			status: ListStatus{
				Name:            c.Name,
				URL:             c.URL,
				Type:            c.Type,
				Format:          c.Format,
				Action:          c.Action,
				Enabled:         c.Enabled,
				RefreshInterval: c.RefreshInterval,
			},
		}
		if ml.cfg.Action == "" {
			ml.cfg.Action = "nxdomain"
		}
		if ml.cfg.Format == "" {
			ml.cfg.Format = "hosts"
		}
		if ml.cfg.Type == "" {
			ml.cfg.Type = "block"
		}
		lm.lists = append(lm.lists, ml)
	}

	return lm
}

// Start performs initial download of all lists and begins periodic refresh goroutines.
func (lm *ListManager) Start(ctx context.Context) {
	ctx, lm.cancel = context.WithCancel(ctx)

	// Initial fetch for all enabled lists
	for i := range lm.lists {
		if !lm.lists[i].cfg.Enabled {
			continue
		}
		lm.refreshList(i)
	}

	// Start refresh goroutines
	for i := range lm.lists {
		if !lm.lists[i].cfg.Enabled {
			continue
		}
		interval := lm.parseInterval(lm.lists[i].cfg.RefreshInterval)
		if interval < 1*time.Minute {
			interval = 24 * time.Hour
		}

		idx := i
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					lm.refreshList(idx)
				}
			}
		}()
	}

	enabled := 0
	total := 0
	for _, ml := range lm.lists {
		if ml.cfg.Enabled {
			enabled++
			total += len(ml.domains)
		}
	}
	lm.logger.Info("DNS list manager started",
		"lists", len(lm.lists),
		"enabled", enabled,
		"total_domains", total)
}

// Stop cancels all refresh goroutines.
func (lm *ListManager) Stop() {
	if lm.cancel != nil {
		lm.cancel()
	}
}

// Check tests a domain against all active lists.
// Returns (blocked bool, action string, listName string).
// Allowlists take priority — if a domain is on any allowlist, it is never blocked.
func (lm *ListManager) Check(qname string) (blocked bool, action string, listName string) {
	domain := strings.ToLower(strings.TrimSuffix(qname, "."))
	if domain == "" {
		return false, "", ""
	}

	lm.mu.RLock()
	defer lm.mu.RUnlock()

	// Check allowlists first
	for i := range lm.lists {
		ml := &lm.lists[i]
		if !ml.cfg.Enabled || ml.cfg.Type != "allow" {
			continue
		}
		if lm.matchDomain(ml, domain) {
			return false, "", ""
		}
	}

	// Check blocklists
	for i := range lm.lists {
		ml := &lm.lists[i]
		if !ml.cfg.Enabled || ml.cfg.Type != "block" {
			continue
		}
		if lm.matchDomain(ml, domain) {
			return true, ml.cfg.Action, ml.cfg.Name
		}
	}

	return false, "", ""
}

// matchDomain checks if a domain or any of its parent domains are in the list.
func (lm *ListManager) matchDomain(ml *managedList, domain string) bool {
	// Exact match
	if _, ok := ml.domains[domain]; ok {
		return true
	}
	// Walk up parent domains (e.g. ads.example.com -> example.com -> com)
	parts := strings.SplitN(domain, ".", 2)
	for len(parts) == 2 && parts[1] != "" {
		if _, ok := ml.domains[parts[1]]; ok {
			return true
		}
		parts = strings.SplitN(parts[1], ".", 2)
	}
	return false
}

// Statuses returns the current status of all lists.
func (lm *ListManager) Statuses() []ListStatus {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]ListStatus, len(lm.lists))
	for i, ml := range lm.lists {
		s := ml.status
		s.DomainCount = len(ml.domains)
		result[i] = s
	}
	return result
}

// RefreshAll forces an immediate refresh of all enabled lists.
func (lm *ListManager) RefreshAll() {
	for i := range lm.lists {
		if lm.lists[i].cfg.Enabled {
			lm.refreshList(i)
		}
	}
}

// RefreshByName forces an immediate refresh of a specific list.
func (lm *ListManager) RefreshByName(name string) error {
	for i := range lm.lists {
		if lm.lists[i].cfg.Name == name {
			lm.refreshList(i)
			return nil
		}
	}
	return fmt.Errorf("list %q not found", name)
}

// TestDomain checks if a specific domain would be blocked and returns details.
func (lm *ListManager) TestDomain(domain string) map[string]interface{} {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	blocked, action, listName := lm.Check(domain + ".")

	result := map[string]interface{}{
		"domain":  domain,
		"blocked": blocked,
	}

	if blocked {
		result["action"] = action
		result["list"] = listName
	}

	// Show which lists contain this domain
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var matches []map[string]interface{}
	for i := range lm.lists {
		ml := &lm.lists[i]
		if !ml.cfg.Enabled {
			continue
		}
		if lm.matchDomain(ml, domain) {
			matches = append(matches, map[string]interface{}{
				"list": ml.cfg.Name,
				"type": ml.cfg.Type,
			})
		}
	}
	result["matches"] = matches

	return result
}

// TotalDomains returns the total number of unique blocked domains across all lists.
func (lm *ListManager) TotalDomains() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	total := 0
	for _, ml := range lm.lists {
		if ml.cfg.Enabled {
			total += len(ml.domains)
		}
	}
	return total
}

// refreshList downloads and parses a single list.
func (lm *ListManager) refreshList(idx int) {
	lm.mu.RLock()
	cfg := lm.lists[idx].cfg
	lm.mu.RUnlock()

	lm.logger.Debug("refreshing DNS list", "name", cfg.Name, "url", cfg.URL)

	req, err := http.NewRequest("GET", cfg.URL, nil)
	if err != nil {
		lm.setError(idx, fmt.Errorf("creating request: %w", err))
		return
	}
	req.Header.Set("User-Agent", "athena-dhcpd/1.0")

	resp, err := lm.client.Do(req)
	if err != nil {
		lm.setError(idx, fmt.Errorf("downloading list: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lm.setError(idx, fmt.Errorf("HTTP %d from %s", resp.StatusCode, cfg.URL))
		return
	}

	domains, err := lm.parseList(resp.Body, cfg.Format)
	if err != nil {
		lm.setError(idx, fmt.Errorf("parsing list: %w", err))
		return
	}

	interval := lm.parseInterval(cfg.RefreshInterval)
	if interval < 1*time.Minute {
		interval = 24 * time.Hour
	}

	lm.mu.Lock()
	lm.lists[idx].domains = domains
	lm.lists[idx].status.LastRefresh = time.Now()
	lm.lists[idx].status.LastError = ""
	lm.lists[idx].status.DomainCount = len(domains)
	lm.lists[idx].status.NextRefresh = time.Now().Add(interval)
	lm.mu.Unlock()

	lm.logger.Info("DNS list refreshed",
		"name", cfg.Name,
		"domains", len(domains),
		"format", cfg.Format)
}

func (lm *ListManager) setError(idx int, err error) {
	lm.mu.Lock()
	lm.lists[idx].status.LastError = err.Error()
	lm.mu.Unlock()
	lm.logger.Warn("DNS list refresh failed",
		"name", lm.lists[idx].cfg.Name,
		"error", err)
}

// parseList reads a list body and returns a set of domains.
func (lm *ListManager) parseList(r io.Reader, format string) (map[string]struct{}, error) {
	domains := make(map[string]struct{})
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		var domain string
		switch format {
		case "hosts":
			domain = parseHostsLine(line)
		case "domains":
			domain = parseDomainsLine(line)
		case "adblock":
			domain = parseAdblockLine(line)
		default:
			domain = parseHostsLine(line)
		}

		if domain != "" {
			domain = strings.ToLower(domain)
			// Skip localhost entries
			if domain == "localhost" || domain == "localhost.localdomain" ||
				domain == "local" || domain == "broadcasthost" {
				continue
			}
			domains[domain] = struct{}{}
		}
	}

	return domains, scanner.Err()
}

// parseHostsLine parses a hosts-file format line: "0.0.0.0 ads.example.com"
func parseHostsLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	// First field is IP (0.0.0.0 or 127.0.0.1), second is domain
	ip := fields[0]
	if ip != "0.0.0.0" && ip != "127.0.0.1" {
		return ""
	}
	domain := fields[1]
	// Strip inline comments
	if idx := strings.IndexByte(domain, '#'); idx >= 0 {
		domain = domain[:idx]
	}
	return strings.TrimSpace(domain)
}

// parseDomainsLine parses a plain domain list: one domain per line.
func parseDomainsLine(line string) string {
	// Strip inline comments
	if idx := strings.IndexByte(line, '#'); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// parseAdblockLine parses adblock-style lines: "||ads.example.com^"
func parseAdblockLine(line string) string {
	if !strings.HasPrefix(line, "||") {
		return ""
	}
	line = strings.TrimPrefix(line, "||")
	// Strip trailing ^ and anything after
	if idx := strings.IndexByte(line, '^'); idx >= 0 {
		line = line[:idx]
	}
	// Skip entries with wildcards or paths
	if strings.ContainsAny(line, "*/$") {
		return ""
	}
	return strings.TrimSpace(line)
}

// BlockResponse creates the appropriate DNS response for a blocked domain.
func BlockResponse(r *dns.Msg, action string) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetReply(r)

	switch action {
	case "nxdomain":
		resp.Rcode = dns.RcodeNameError
	case "zero":
		// Return 0.0.0.0 for A queries, :: for AAAA
		if len(r.Question) > 0 {
			q := r.Question[0]
			switch q.Qtype {
			case dns.TypeA:
				resp.Answer = []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
						A:   []byte{0, 0, 0, 0},
					},
				}
			case dns.TypeAAAA:
				resp.Answer = []dns.RR{
					&dns.AAAA{
						Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300},
						AAAA: make([]byte, 16),
					},
				}
			default:
				resp.Rcode = dns.RcodeNameError
			}
		}
	case "refuse":
		resp.Rcode = dns.RcodeRefused
	default:
		resp.Rcode = dns.RcodeNameError
	}

	return resp
}

func (lm *ListManager) parseInterval(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d < 1*time.Minute {
		return 24 * time.Hour
	}
	return d
}
