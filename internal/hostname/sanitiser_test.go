package hostname

import (
	"log/slog"
	"net"
	"os"
	"testing"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func noopLookup(hostname, subnet string, mac net.HardwareAddr) bool {
	return false
}

func defaultCfg() config.HostnameSanitisationConfig {
	return config.HostnameSanitisationConfig{
		Enabled:     true,
		StripEmoji:  true,
		Lowercase:   true,
		DedupSuffix: true,
		MaxLength:   63,
	}
}

func mustMAC(s string) net.HardwareAddr {
	m, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func TestBasicSanitise(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"clean", "myhost", "myhost"},
		{"uppercase", "MyHost", "myhost"},
		{"spaces", "my host", "myhost"},
		{"special chars", "my@host!name", "myhostname"},
		{"leading dots", "..myhost", "myhost"},
		{"trailing hyphens", "myhost--", "myhost"},
		{"unicode", "hôst-nàme", "hst-nme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := basicSanitise(tt.in)
			if got != tt.want {
				t.Errorf("basicSanitise(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripControlChars(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"hello\x00world", "helloworld"},
		{"tab\there", "tabhere"},
		{"\x01\x02\x03", ""},
		{"ok\x7fno", "okno"},
	}
	for _, tt := range tests {
		got := stripControlChars(tt.in)
		if got != tt.want {
			t.Errorf("stripControlChars(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripInvalidDNS(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"valid-host.name", "valid-host.name"},
		{"has spaces", "hasspaces"},
		{"sql' OR 1=1;--", "sqlOR11--"},
		{"under_score", "underscore"},
		{"café", "caf"},
	}
	for _, tt := range tests {
		got := stripInvalidDNS(tt.in)
		if got != tt.want {
			t.Errorf("stripInvalidDNS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCollapseRepeated(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"a..b", "a.b"},
		{"a--b", "a-b"},
		{"a...b---c", "a.b-c"},
		{"normal", "normal"},
	}
	for _, tt := range tests {
		got := collapseRepeated(tt.in)
		if got != tt.want {
			t.Errorf("collapseRepeated(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitiserDisabled(t *testing.T) {
	cfg := config.HostnameSanitisationConfig{Enabled: false}
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	// Should fall back to basicSanitise behaviour
	got, modified := s.Sanitise("MyHost", "10.0.0.0/24", mustMAC("aa:bb:cc:dd:ee:ff"))
	if got != "myhost" {
		t.Errorf("disabled sanitiser got %q, want %q", got, "myhost")
	}
	if !modified {
		t.Error("expected modified=true for case change")
	}
}

func TestSanitiserBuiltinDeny(t *testing.T) {
	cfg := defaultCfg()
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	denied := []string{"localhost", "LOCALHOST", "unknown", "none", "null", "test", "default", "iphone", "ipad"}
	for _, h := range denied {
		got, _ := s.Sanitise(h, "10.0.0.0/24", mac)
		if got == "" || got == h {
			t.Errorf("expected %q to be denied and replaced, got %q", h, got)
		}
		// Should get a MAC-based fallback
		if got != "dhcp-aabbccddeeff" {
			t.Errorf("denied %q → %q, expected MAC fallback dhcp-aabbccddeeff", h, got)
		}
	}
}

func TestSanitiserAndroidPattern(t *testing.T) {
	cfg := defaultCfg()
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("11:22:33:44:55:66")

	got, _ := s.Sanitise("android-abcdef123456", "10.0.0.0/24", mac)
	if got != "dhcp-112233445566" {
		t.Errorf("android pattern got %q, want MAC fallback", got)
	}
}

func TestSanitiserUserDenyPattern(t *testing.T) {
	cfg := defaultCfg()
	cfg.DenyPatterns = []string{`^printer-`, `^kiosk\d+$`}
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	tests := []struct {
		in     string
		denied bool
	}{
		{"printer-floor2", true},
		{"kiosk42", true},
		{"workstation-1", false},
	}
	for _, tt := range tests {
		got, _ := s.Sanitise(tt.in, "10.0.0.0/24", mac)
		wasDenied := (got == "dhcp-aabbccddeeff")
		if wasDenied != tt.denied {
			t.Errorf("Sanitise(%q): denied=%v, want denied=%v (got %q)", tt.in, wasDenied, tt.denied, got)
		}
	}
}

func TestSanitiserAllowRegex(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowRegex = `^[a-z]+-\d+$`
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	tests := []struct {
		in    string
		allow bool
	}{
		{"server-1", true},
		{"ap-42", true},
		{"random", false},
		{"123-abc", false},
	}
	for _, tt := range tests {
		got, _ := s.Sanitise(tt.in, "10.0.0.0/24", mac)
		allowed := (got == tt.in)
		if allowed != tt.allow {
			t.Errorf("Sanitise(%q) allow=%v, want allow=%v (got %q)", tt.in, allowed, tt.allow, got)
		}
	}
}

func TestSanitiserMaxLength(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxLength = 10
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	got, _ := s.Sanitise("this-is-a-very-long-hostname", "10.0.0.0/24", mac)
	if len(got) > 10 {
		t.Errorf("length %d exceeds max 10: %q", len(got), got)
	}
}

func TestSanitiserEmoji(t *testing.T) {
	cfg := defaultCfg()
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	got, _ := s.Sanitise("my🔥host💻", "10.0.0.0/24", mac)
	if got != "myhost" {
		t.Errorf("emoji strip got %q, want %q", got, "myhost")
	}
}

func TestSanitiserDedup(t *testing.T) {
	cfg := defaultCfg()

	// Simulate "server-1" being taken by a different MAC
	takenMAC := mustMAC("11:22:33:44:55:66")
	lookup := func(hostname, subnet string, mac net.HardwareAddr) bool {
		if hostname == "server-1" && mac.String() != takenMAC.String() {
			return true
		}
		return false
	}

	s, err := NewSanitiser(cfg, nil, lookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	newMAC := mustMAC("aa:bb:cc:dd:ee:ff")
	got, _ := s.Sanitise("server-1", "10.0.0.0/24", newMAC)
	if got != "server-1-2" {
		t.Errorf("dedup got %q, want %q", got, "server-1-2")
	}

	// Same MAC should not trigger dedup
	got2, _ := s.Sanitise("server-1", "10.0.0.0/24", takenMAC)
	if got2 != "server-1" {
		t.Errorf("same MAC dedup got %q, want %q", got2, "server-1")
	}
}

func TestSanitiserPerSubnetOverride(t *testing.T) {
	global := defaultCfg()
	global.DenyPatterns = []string{`^blocked$`}

	subnetCfg := config.HostnameSanitisationConfig{
		Enabled:      true,
		Lowercase:    true,
		StripEmoji:   true,
		DedupSuffix:  false,
		DenyPatterns: nil, // No deny patterns on this subnet
	}

	subnets := []config.SubnetConfig{
		{
			Network:              "10.0.0.0/24",
			HostnameSanitisation: &subnetCfg,
		},
	}

	s, err := NewSanitiser(global, subnets, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	// "blocked" should be denied on other subnets (global config)
	got1, _ := s.Sanitise("blocked", "192.168.0.0/24", mac)
	if got1 != "dhcp-aabbccddeeff" {
		t.Errorf("global deny: got %q, want MAC fallback", got1)
	}

	// "blocked" should be allowed on overridden subnet (no deny patterns)
	got2, _ := s.Sanitise("blocked", "10.0.0.0/24", mac)
	if got2 != "blocked" {
		t.Errorf("subnet override: got %q, want %q", got2, "blocked")
	}
}

func TestSanitiserFallbackTemplate(t *testing.T) {
	cfg := defaultCfg()
	cfg.FallbackTemplate = "client-{mac}"
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	// Force rejection via built-in deny
	got, _ := s.Sanitise("localhost", "10.0.0.0/24", mac)
	if got != "client-aabbccddeeff" {
		t.Errorf("fallback template got %q, want %q", got, "client-aabbccddeeff")
	}
}

func TestSanitiserCleanInput(t *testing.T) {
	cfg := defaultCfg()
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	// Normal hostname should pass through untouched (after lowercasing)
	got, _ := s.Sanitise("workstation-42", "10.0.0.0/24", mac)
	if got != "workstation-42" {
		t.Errorf("clean input got %q, want %q", got, "workstation-42")
	}
}

func TestSanitiserSQLInjection(t *testing.T) {
	cfg := defaultCfg()
	s, err := NewSanitiser(cfg, nil, noopLookup, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	mac := mustMAC("aa:bb:cc:dd:ee:ff")

	got, _ := s.Sanitise("'; DROP TABLE leases;--", "10.0.0.0/24", mac)
	// All special chars stripped, should just be clean alpha
	if got == "'; DROP TABLE leases;--" {
		t.Errorf("SQL injection not sanitised: %q", got)
	}
	// Result should only contain [a-z0-9.-]
	for _, c := range got {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.') {
			t.Errorf("invalid char %q in sanitised result %q", string(c), got)
		}
	}
}
