// Package hostname provides a configurable hostname sanitisation pipeline
// for cleaning client-supplied hostnames before DNS registration.
// Clients send garbage in option 12 — emoji, spaces, control characters,
// SQL injection attempts, duplicates, "localhost", "android-abc123def".
// This pipeline strips, validates, deduplicates, and rewrites hostnames
// before they reach DNS.
package hostname

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// HostnameLookupFunc checks whether a hostname is already in use.
// Returns true if the hostname is taken by a different MAC.
type HostnameLookupFunc func(hostname, subnet, mac string) bool

// compiledConfig holds a config alongside its compiled regex patterns.
type compiledConfig struct {
	cfg          config.HostnameSanitisationConfig
	allowRegex   *regexp.Regexp
	denyPatterns []*regexp.Regexp
}

// Sanitiser applies a configurable pipeline to client-supplied hostnames.
type Sanitiser struct {
	global  compiledConfig
	subnets map[string]*compiledConfig // network → per-subnet compiled override
	lookup  HostnameLookupFunc
	logger  *slog.Logger
	mu      sync.RWMutex

	// well-known bad hostnames that should never be registered in DNS
	builtinDeny []*regexp.Regexp
}

// NewSanitiser creates a new hostname sanitiser from config.
func NewSanitiser(global config.HostnameSanitisationConfig, subnets []config.SubnetConfig, lookup HostnameLookupFunc, logger *slog.Logger) (*Sanitiser, error) {
	globalCompiled, err := compileConfig(global)
	if err != nil {
		return nil, fmt.Errorf("compiling global hostname config: %w", err)
	}

	s := &Sanitiser{
		global:  *globalCompiled,
		subnets: make(map[string]*compiledConfig),
		lookup:  lookup,
		logger:  logger,
	}

	// Compile per-subnet overrides
	for i := range subnets {
		if subnets[i].HostnameSanitisation != nil {
			cc, err := compileConfig(*subnets[i].HostnameSanitisation)
			if err != nil {
				return nil, fmt.Errorf("compiling hostname config for subnet %s: %w", subnets[i].Network, err)
			}
			s.subnets[subnets[i].Network] = cc
		}
	}

	// Built-in deny patterns for known garbage hostnames
	builtinPatterns := []string{
		`^localhost$`,
		`^localhost\.localdomain$`,
		`^android-[a-f0-9]{12,}$`,
		`^galaxy-[a-f0-9]+$`,
		`^iphone$`,
		`^ipad$`,
		`^host$`,
		`^dhcp$`,
		`^unknown$`,
		`^none$`,
		`^null$`,
		`^test$`,
		`^default$`,
		`^changeme$`,
		`^\*$`,
		`^_$`,
	}
	for _, p := range builtinPatterns {
		s.builtinDeny = append(s.builtinDeny, regexp.MustCompile("(?i)"+p))
	}

	return s, nil
}

// Sanitise runs the full pipeline on a hostname.
// Returns the cleaned hostname (may be empty if rejected) and whether it was modified.
func (s *Sanitiser) Sanitise(hostname, subnet, mac string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cc := s.effectiveCompiled(subnet)
	cfg := cc.cfg
	if !cfg.Enabled {
		// Even when disabled, do basic cleanup (legacy behaviour)
		cleaned := basicSanitise(hostname)
		return cleaned, cleaned != hostname
	}

	original := hostname

	// Step 1: strip non-printable / control characters
	hostname = stripControlChars(hostname)

	// Step 2: strip emoji if configured
	if cfg.StripEmoji {
		hostname = stripEmoji(hostname)
	}

	// Step 3: strip characters invalid for DNS labels (RFC 952 / RFC 1123)
	hostname = stripInvalidDNS(hostname)

	// Step 4: lowercase if configured
	if cfg.Lowercase {
		hostname = strings.ToLower(hostname)
	}

	// Step 5: trim leading/trailing hyphens and dots
	hostname = strings.Trim(hostname, ".-")

	// Step 6: collapse consecutive dots/hyphens
	hostname = collapseRepeated(hostname)

	// Step 7: enforce max length
	maxLen := cfg.MaxLength
	if maxLen <= 0 {
		maxLen = 63 // DNS label limit
	}
	if len(hostname) > maxLen {
		hostname = hostname[:maxLen]
		hostname = strings.TrimRight(hostname, ".-")
	}

	// Step 8: check against built-in deny patterns
	if s.matchesBuiltinDeny(hostname) {
		s.logger.Debug("hostname rejected by built-in deny",
			"original", original, "cleaned", hostname, "mac", mac)
		hostname = s.fallback(cfg, mac)
		return hostname, true
	}

	// Step 9: check against user deny patterns (per-subnet aware)
	if matchesDenyPatterns(hostname, cc.denyPatterns) {
		s.logger.Debug("hostname rejected by deny pattern",
			"original", original, "cleaned", hostname, "mac", mac)
		hostname = s.fallback(cfg, mac)
		return hostname, true
	}

	// Step 10: check against allow regex (per-subnet aware; if set, hostname must match)
	if cc.allowRegex != nil && !cc.allowRegex.MatchString(hostname) {
		s.logger.Debug("hostname rejected by allow regex",
			"original", original, "cleaned", hostname, "mac", mac,
			"regex", cfg.AllowRegex)
		hostname = s.fallback(cfg, mac)
		return hostname, true
	}

	// Step 11: empty after cleanup — use fallback
	if hostname == "" {
		hostname = s.fallback(cfg, mac)
		return hostname, true
	}

	// Step 12: deduplication
	if cfg.DedupSuffix && s.lookup != nil && s.lookup(hostname, subnet, mac) {
		hostname = s.deduplicate(hostname, subnet, mac, maxLen)
	}

	return hostname, hostname != original
}

// effectiveCompiled returns the per-subnet compiled config if it exists, otherwise global.
func (s *Sanitiser) effectiveCompiled(subnet string) *compiledConfig {
	if sub, ok := s.subnets[subnet]; ok {
		return sub
	}
	return &s.global
}

// fallback generates a fallback hostname from the MAC address.
func (s *Sanitiser) fallback(cfg config.HostnameSanitisationConfig, mac string) string {
	tmpl := cfg.FallbackTemplate
	if tmpl == "" {
		tmpl = "dhcp-{mac}"
	}

	macStr := strings.ReplaceAll(mac, ":", "")
	result := strings.ReplaceAll(tmpl, "{mac}", macStr)
	return result
}

// deduplicate appends -2, -3, etc. until a unique hostname is found.
func (s *Sanitiser) deduplicate(hostname, subnet, mac string, maxLen int) string {
	for i := 2; i <= 99; i++ {
		suffix := fmt.Sprintf("-%d", i)
		candidate := hostname
		// Truncate base to fit the suffix within maxLen
		if len(candidate)+len(suffix) > maxLen {
			candidate = candidate[:maxLen-len(suffix)]
			candidate = strings.TrimRight(candidate, ".-")
		}
		candidate = candidate + suffix
		if !s.lookup(candidate, subnet, mac) {
			return candidate
		}
	}
	// Exhausted — use MAC fallback
	macStr := strings.ReplaceAll(mac, ":", "")
	return "dhcp-" + macStr
}

// matchesBuiltinDeny returns true if hostname matches any built-in deny pattern.
func (s *Sanitiser) matchesBuiltinDeny(hostname string) bool {
	for _, re := range s.builtinDeny {
		if re.MatchString(hostname) {
			return true
		}
	}
	return false
}

// matchesDenyPatterns returns true if hostname matches any of the given deny patterns.
func matchesDenyPatterns(hostname string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(hostname) {
			return true
		}
	}
	return false
}

// UpdateConfig updates the sanitiser configuration (for hot-reload).
func (s *Sanitiser) UpdateConfig(global config.HostnameSanitisationConfig, subnets []config.SubnetConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	globalCompiled, err := compileConfig(global)
	if err != nil {
		return fmt.Errorf("compiling global hostname config: %w", err)
	}
	s.global = *globalCompiled

	s.subnets = make(map[string]*compiledConfig)
	for i := range subnets {
		if subnets[i].HostnameSanitisation != nil {
			cc, err := compileConfig(*subnets[i].HostnameSanitisation)
			if err != nil {
				return fmt.Errorf("compiling hostname config for subnet %s: %w", subnets[i].Network, err)
			}
			s.subnets[subnets[i].Network] = cc
		}
	}

	return nil
}

// compileConfig compiles regex patterns from a HostnameSanitisationConfig.
func compileConfig(cfg config.HostnameSanitisationConfig) (*compiledConfig, error) {
	cc := &compiledConfig{cfg: cfg}

	if cfg.AllowRegex != "" {
		re, err := regexp.Compile(cfg.AllowRegex)
		if err != nil {
			return nil, fmt.Errorf("compiling allow_regex %q: %w", cfg.AllowRegex, err)
		}
		cc.allowRegex = re
	}

	for _, p := range cfg.DenyPatterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("compiling deny_pattern %q: %w", p, err)
		}
		cc.denyPatterns = append(cc.denyPatterns, re)
	}

	return cc, nil
}

// --- low-level helpers ---

// basicSanitise is the minimal cleanup used when the pipeline is disabled.
// Matches the legacy SanitizeHostname behaviour.
func basicSanitise(hostname string) string {
	if hostname == "" {
		return ""
	}
	var result []byte
	for _, c := range []byte(hostname) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.' {
			result = append(result, c)
		}
	}
	s := string(result)
	s = strings.Trim(s, ".-")
	if len(s) > 253 {
		s = s[:253]
	}
	return strings.ToLower(s)
}

// stripControlChars removes ASCII control characters and non-printable runes.
func stripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 32 && r != 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripEmoji removes emoji and other symbol runes.
func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isEmoji(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isEmoji returns true for common emoji/symbol Unicode ranges.
func isEmoji(r rune) bool {
	if r < 128 {
		return false
	}
	return unicode.Is(unicode.So, r) || // Other_Symbol (most emoji)
		unicode.Is(unicode.Sk, r) || // Modifier_Symbol
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map Symbols
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols
		(r >= 0x2600 && r <= 0x26FF) || // Misc Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0xFE00 && r <= 0xFE0F) || // Variation Selectors
		(r >= 0x200D && r <= 0x200D) // Zero-width joiner
}

// stripInvalidDNS removes characters not valid in DNS labels (RFC 952/1123).
// Valid: a-z, A-Z, 0-9, hyphen, dot.
func stripInvalidDNS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// collapseRepeated collapses consecutive dots or hyphens into single instances.
func collapseRepeated(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var prev byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == '.' || c == '-') && c == prev {
			continue
		}
		b.WriteByte(c)
		prev = c
	}
	return b.String()
}
