package dnsproxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
	"github.com/miekg/dns"
)

func testListLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestParseHostsLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"0.0.0.0 ads.example.com", "ads.example.com"},
		{"127.0.0.1 tracker.example.com", "tracker.example.com"},
		{"0.0.0.0 ads.example.com # comment", "ads.example.com"},
		{"# this is a comment", ""},
		{"10.0.0.1 myhost.local", ""},
		{"0.0.0.0 localhost", "localhost"},
		{"", ""},
		{"0.0.0.0", ""},
	}

	for _, tt := range tests {
		got := parseHostsLine(tt.line)
		if got != tt.want {
			t.Errorf("parseHostsLine(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestParseDomainsLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"ads.example.com", "ads.example.com"},
		{"ads.example.com # inline comment", "ads.example.com"},
		{"  whitespace.com  ", "whitespace.com"},
		{"", ""},
	}

	for _, tt := range tests {
		got := parseDomainsLine(tt.line)
		if got != tt.want {
			t.Errorf("parseDomainsLine(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestParseAdblockLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"||ads.example.com^", "ads.example.com"},
		{"||tracker.net^$third-party", "tracker.net"},
		{"||simple.org^", "simple.org"},
		{"! comment line", ""},
		{"@@||allowlisted.com^", ""},
		{"||wild*.example.com^", ""},
		{"not-a-rule", ""},
		{"||", ""},
	}

	for _, tt := range tests {
		got := parseAdblockLine(tt.line)
		if got != tt.want {
			t.Errorf("parseAdblockLine(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestListManagerParseList(t *testing.T) {
	lm := NewListManager(nil, testListLogger())

	tests := []struct {
		name    string
		format  string
		input   string
		wantLen int
	}{
		{
			"hosts format",
			"hosts",
			"0.0.0.0 ads.example.com\n0.0.0.0 tracker.example.com\n# comment\n0.0.0.0 localhost\n",
			2,
		},
		{
			"domains format",
			"domains",
			"ads.example.com\ntracker.example.com\n# comment\n\n",
			2,
		},
		{
			"adblock format",
			"adblock",
			"! title\n||ads.example.com^\n||tracker.net^\n||wild*.bad.com^\n",
			2,
		},
		{
			"empty input",
			"hosts",
			"",
			0,
		},
		{
			"all comments",
			"hosts",
			"# comment 1\n# comment 2\n",
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domains, err := lm.parseList(strings.NewReader(tt.input), tt.format)
			if err != nil {
				t.Fatalf("parseList error: %v", err)
			}
			if len(domains) != tt.wantLen {
				t.Errorf("got %d domains, want %d (domains: %v)", len(domains), tt.wantLen, domains)
			}
		})
	}
}

func TestListManagerCheck(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "blocklist", URL: "http://test", Type: "block", Format: "domains", Action: "nxdomain", Enabled: true},
	}, testListLogger())

	// Manually populate the blocklist
	lm.mu.Lock()
	lm.lists[0].domains = map[string]struct{}{
		"ads.example.com":     {},
		"tracker.example.com": {},
		"malware.net":         {},
	}
	lm.mu.Unlock()

	tests := []struct {
		qname   string
		blocked bool
		action  string
	}{
		{"ads.example.com.", true, "nxdomain"},
		{"tracker.example.com.", true, "nxdomain"},
		{"safe.example.com.", false, ""},
		{"sub.ads.example.com.", true, "nxdomain"},      // subdomain match
		{"deep.sub.ads.example.com.", true, "nxdomain"}, // deep subdomain
		{"notads.example.com.", false, ""},              // different subdomain
		{"malware.net.", true, "nxdomain"},
		{".", false, ""},
		{"", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.qname, func(t *testing.T) {
			blocked, action, _ := lm.Check(tt.qname)
			if blocked != tt.blocked {
				t.Errorf("Check(%q) blocked = %v, want %v", tt.qname, blocked, tt.blocked)
			}
			if action != tt.action {
				t.Errorf("Check(%q) action = %q, want %q", tt.qname, action, tt.action)
			}
		})
	}
}

func TestListManagerAllowlistPriority(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "blocklist", URL: "http://test", Type: "block", Format: "domains", Action: "nxdomain", Enabled: true},
		{Name: "allowlist", URL: "http://test", Type: "allow", Format: "domains", Enabled: true},
	}, testListLogger())

	// Populate both lists
	lm.mu.Lock()
	lm.lists[0].domains = map[string]struct{}{
		"ads.example.com":  {},
		"good.example.com": {},
	}
	lm.lists[1].domains = map[string]struct{}{
		"good.example.com": {}, // also on allowlist
	}
	lm.mu.Unlock()

	// ads.example.com should be blocked
	blocked, _, _ := lm.Check("ads.example.com.")
	if !blocked {
		t.Error("ads.example.com should be blocked")
	}

	// good.example.com is on both lists, allowlist wins
	blocked, _, _ = lm.Check("good.example.com.")
	if blocked {
		t.Error("good.example.com should NOT be blocked (allowlist priority)")
	}
}

func TestListManagerDisabledList(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "disabled", URL: "http://test", Type: "block", Format: "domains", Action: "nxdomain", Enabled: false},
	}, testListLogger())

	lm.mu.Lock()
	lm.lists[0].domains = map[string]struct{}{
		"ads.example.com": {},
	}
	lm.mu.Unlock()

	blocked, _, _ := lm.Check("ads.example.com.")
	if blocked {
		t.Error("disabled list should not block anything")
	}
}

func TestListManagerActions(t *testing.T) {
	tests := []struct {
		action string
		list   string
	}{
		{"nxdomain", "nxdomain-list"},
		{"zero", "zero-list"},
		{"refuse", "refuse-list"},
	}

	for _, tt := range tests {
		lm := NewListManager([]config.DNSListConfig{
			{Name: tt.list, URL: "http://test", Type: "block", Format: "domains", Action: tt.action, Enabled: true},
		}, testListLogger())

		lm.mu.Lock()
		lm.lists[0].domains = map[string]struct{}{"blocked.com": {}}
		lm.mu.Unlock()

		_, action, name := lm.Check("blocked.com.")
		if action != tt.action {
			t.Errorf("action = %q, want %q", action, tt.action)
		}
		if name != tt.list {
			t.Errorf("list name = %q, want %q", name, tt.list)
		}
	}
}

func TestBlockResponse(t *testing.T) {
	tests := []struct {
		action string
		qtype  uint16
		rcode  int
		hasAns bool
	}{
		{"nxdomain", dns.TypeA, dns.RcodeNameError, false},
		{"refuse", dns.TypeA, dns.RcodeRefused, false},
		{"zero", dns.TypeA, dns.RcodeSuccess, true},
		{"zero", dns.TypeAAAA, dns.RcodeSuccess, true},
		{"zero", dns.TypeMX, dns.RcodeNameError, false},
		{"unknown", dns.TypeA, dns.RcodeNameError, false},
	}

	for _, tt := range tests {
		t.Run(tt.action+"_"+dns.TypeToString[tt.qtype], func(t *testing.T) {
			r := new(dns.Msg)
			r.SetQuestion("blocked.example.com.", tt.qtype)

			resp := BlockResponse(r, tt.action)
			if resp.Rcode != tt.rcode {
				t.Errorf("rcode = %d, want %d", resp.Rcode, tt.rcode)
			}
			if tt.hasAns && len(resp.Answer) == 0 {
				t.Error("expected answer records")
			}
			if !tt.hasAns && len(resp.Answer) > 0 {
				t.Error("expected no answer records")
			}
		})
	}
}

func TestBlockResponseZeroIP(t *testing.T) {
	r := new(dns.Msg)
	r.SetQuestion("blocked.com.", dns.TypeA)
	resp := BlockResponse(r, "zero")

	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("expected A record")
	}
	if a.A.String() != "0.0.0.0" {
		t.Errorf("expected 0.0.0.0, got %s", a.A.String())
	}
}

func TestListManagerTestDomain(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "ads", URL: "http://test", Type: "block", Format: "domains", Action: "nxdomain", Enabled: true},
	}, testListLogger())

	lm.mu.Lock()
	lm.lists[0].domains = map[string]struct{}{"ads.example.com": {}}
	lm.mu.Unlock()

	result := lm.TestDomain("ads.example.com")
	if result["blocked"] != true {
		t.Error("TestDomain should report blocked")
	}

	result = lm.TestDomain("safe.example.com")
	if result["blocked"] != false {
		t.Error("TestDomain should report not blocked")
	}
}

func TestListManagerStatuses(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "list-a", URL: "http://a", Type: "block", Enabled: true, RefreshInterval: "1h"},
		{Name: "list-b", URL: "http://b", Type: "allow", Enabled: false, RefreshInterval: "24h"},
	}, testListLogger())

	statuses := lm.Statuses()
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2", len(statuses))
	}
	if statuses[0].Name != "list-a" {
		t.Errorf("status[0].Name = %q", statuses[0].Name)
	}
	if statuses[1].Enabled != false {
		t.Error("status[1] should be disabled")
	}
}

func TestListManagerTotalDomains(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "a", Type: "block", Enabled: true},
		{Name: "b", Type: "block", Enabled: true},
		{Name: "c", Type: "block", Enabled: false},
	}, testListLogger())

	lm.mu.Lock()
	lm.lists[0].domains = map[string]struct{}{"a.com": {}, "b.com": {}}
	lm.lists[1].domains = map[string]struct{}{"c.com": {}}
	lm.lists[2].domains = map[string]struct{}{"d.com": {}, "e.com": {}, "f.com": {}}
	lm.mu.Unlock()

	total := lm.TotalDomains()
	if total != 3 {
		t.Errorf("TotalDomains() = %d, want 3 (disabled list excluded)", total)
	}
}

func TestListManagerRefreshByName(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "exists", Enabled: true},
	}, testListLogger())

	err := lm.RefreshByName("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent list")
	}
}

func TestListManagerRefreshFromHTTP(t *testing.T) {
	// Serve a test blocklist
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0.0.0.0 ads.test.com\n0.0.0.0 tracker.test.com\n# comment\n"))
	}))
	defer srv.Close()

	lm := NewListManager([]config.DNSListConfig{
		{Name: "test-http", URL: srv.URL, Type: "block", Format: "hosts", Action: "nxdomain", Enabled: true, RefreshInterval: "1h"},
	}, testListLogger())

	// Manually trigger refresh
	lm.refreshList(0)

	if lm.lists[0].status.LastError != "" {
		t.Fatalf("refresh error: %s", lm.lists[0].status.LastError)
	}

	if len(lm.lists[0].domains) != 2 {
		t.Errorf("got %d domains, want 2", len(lm.lists[0].domains))
	}

	// Verify blocking works
	blocked, _, _ := lm.Check("ads.test.com.")
	if !blocked {
		t.Error("ads.test.com should be blocked after refresh")
	}

	blocked, _, _ = lm.Check("safe.test.com.")
	if blocked {
		t.Error("safe.test.com should not be blocked")
	}
}

func TestListManagerRefreshHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	lm := NewListManager([]config.DNSListConfig{
		{Name: "bad-url", URL: srv.URL, Type: "block", Format: "hosts", Enabled: true},
	}, testListLogger())

	lm.refreshList(0)

	if lm.lists[0].status.LastError == "" {
		t.Error("expected error after 404 response")
	}
}

func TestListManagerParseInterval(t *testing.T) {
	lm := NewListManager(nil, testListLogger())

	tests := []struct {
		input string
		want  string
	}{
		{"1h", "1h0m0s"},
		{"24h", "24h0m0s"},
		{"30m", "30m0s"},
		{"invalid", "24h0m0s"},
		{"", "24h0m0s"},
		{"10s", "24h0m0s"}, // too short, defaults to 24h
	}

	for _, tt := range tests {
		got := lm.parseInterval(tt.input)
		if got.String() != tt.want {
			t.Errorf("parseInterval(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestListManagerDefaultValues(t *testing.T) {
	lm := NewListManager([]config.DNSListConfig{
		{Name: "defaults-test", URL: "http://test", Enabled: true},
	}, testListLogger())

	ml := lm.lists[0]
	if ml.cfg.Action != "nxdomain" {
		t.Errorf("default action = %q, want nxdomain", ml.cfg.Action)
	}
	if ml.cfg.Format != "hosts" {
		t.Errorf("default format = %q, want hosts", ml.cfg.Format)
	}
	if ml.cfg.Type != "block" {
		t.Errorf("default type = %q, want block", ml.cfg.Type)
	}
}
