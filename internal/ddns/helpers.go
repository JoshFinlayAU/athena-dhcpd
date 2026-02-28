package ddns

import (
	"fmt"
	"net"
	"strings"
)

// ensureDot ensures a DNS name ends with a trailing dot.
func ensureDot(name string) string {
	if name == "" {
		return ""
	}
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// ReverseIPName converts an IPv4 address to its in-addr.arpa PTR name.
// e.g., 192.168.1.100 → 100.1.168.192.in-addr.arpa
func ReverseIPName(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", ip4[3], ip4[2], ip4[1], ip4[0])
}

// BuildFQDN constructs an FQDN from hostname and domain.
// Priority: client option 81 → hostname+domain → MAC fallback → empty.
func BuildFQDN(clientFQDN, hostname, domain string, mac string, fallbackToMAC bool) string {
	// Client-provided FQDN (option 81)
	if clientFQDN != "" {
		return ensureDot(clientFQDN)
	}

	// Hostname + domain
	if hostname != "" && domain != "" {
		return ensureDot(hostname + "." + domain)
	}
	if hostname != "" {
		return hostname
	}

	// MAC fallback
	if fallbackToMAC && mac != "" {
		macStr := strings.ReplaceAll(mac, ":", "-")
		if domain != "" {
			return ensureDot(macStr + "." + domain)
		}
		return macStr
	}

	return ""
}

// SanitizeHostname cleans a hostname for DNS use.
// Removes invalid characters and enforces length limits.
func SanitizeHostname(hostname string) string {
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
	// Remove leading/trailing dots and hyphens
	s = strings.Trim(s, ".-")

	// Enforce 253 char limit for FQDN
	if len(s) > 253 {
		s = s[:253]
	}

	return strings.ToLower(s)
}

// DNSUpdater is the interface for DNS update backends.
type DNSUpdater interface {
	AddA(zone, fqdn string, ip net.IP, ttl uint32) error
	RemoveA(zone, fqdn string) error
	AddPTR(zone, reverseIP, fqdn string, ttl uint32) error
	RemovePTR(zone, reverseIP string) error
}
