// Package dnsproxy provides a built-in DNS proxy with DHCP lease registration.
package dnsproxy

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// Record is a single DNS record in the local zone.
type Record struct {
	Name  string
	Type  uint16
	Value string
	TTL   uint32
}

// Zone holds local DNS records — DHCP lease registrations and static entries.
// All lookups are O(1) via maps keyed by lowercase FQDN+type.
type Zone struct {
	mu      sync.RWMutex
	records map[string][]dns.RR // key: "name|type" e.g. "host.example.com.|1"
	domain  string              // default domain suffix e.g. "example.com."
	ttl     uint32
}

// NewZone creates an empty local zone.
func NewZone(domain string, ttl uint32) *Zone {
	if domain != "" && !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
	return &Zone{
		records: make(map[string][]dns.RR),
		domain:  dns.Fqdn(domain),
		ttl:     ttl,
	}
}

// Domain returns the zone's domain suffix.
func (z *Zone) Domain() string {
	return z.domain
}

func recordKey(name string, qtype uint16) string {
	return strings.ToLower(dns.Fqdn(name)) + "|" + fmt.Sprintf("%d", qtype)
}

// Add inserts or replaces a record in the zone.
func (z *Zone) Add(rr dns.RR) {
	z.mu.Lock()
	defer z.mu.Unlock()

	name := strings.ToLower(rr.Header().Name)
	key := recordKey(name, rr.Header().Rrtype)

	// Replace existing records with same name+type (for lease updates)
	z.records[key] = []dns.RR{rr}
}

// AddMulti appends a record without replacing existing ones of the same type.
func (z *Zone) AddMulti(rr dns.RR) {
	z.mu.Lock()
	defer z.mu.Unlock()

	name := strings.ToLower(rr.Header().Name)
	key := recordKey(name, rr.Header().Rrtype)
	z.records[key] = append(z.records[key], rr)
}

// Remove deletes all records matching name+type.
func (z *Zone) Remove(name string, qtype uint16) {
	z.mu.Lock()
	defer z.mu.Unlock()
	delete(z.records, recordKey(name, qtype))
}

// RemoveByValue deletes a specific record by name+type+value.
func (z *Zone) RemoveByValue(name string, qtype uint16, value string) {
	z.mu.Lock()
	defer z.mu.Unlock()

	key := recordKey(name, qtype)
	existing := z.records[key]
	if len(existing) == 0 {
		return
	}

	var kept []dns.RR
	for _, rr := range existing {
		if rrValue(rr) != value {
			kept = append(kept, rr)
		}
	}
	if len(kept) == 0 {
		delete(z.records, key)
	} else {
		z.records[key] = kept
	}
}

// Lookup returns matching records for a query.
func (z *Zone) Lookup(name string, qtype uint16) []dns.RR {
	z.mu.RLock()
	defer z.mu.RUnlock()

	key := recordKey(name, qtype)
	rrs := z.records[key]
	if len(rrs) == 0 {
		return nil
	}

	// Return copies with current TTL
	result := make([]dns.RR, len(rrs))
	for i, rr := range rrs {
		result[i] = dns.Copy(rr)
	}
	return result
}

// Has returns true if any records exist for name+type.
func (z *Zone) Has(name string, qtype uint16) bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return len(z.records[recordKey(name, qtype)]) > 0
}

// AllRecords returns all records in the zone as a flat slice.
func (z *Zone) AllRecords() []dns.RR {
	z.mu.RLock()
	defer z.mu.RUnlock()

	var all []dns.RR
	for _, rrs := range z.records {
		for _, rr := range rrs {
			all = append(all, dns.Copy(rr))
		}
	}
	return all
}

// Count returns total record count.
func (z *Zone) Count() int {
	z.mu.RLock()
	defer z.mu.RUnlock()
	total := 0
	for _, rrs := range z.records {
		total += len(rrs)
	}
	return total
}

// RegisterLease adds A and optional PTR records for a DHCP lease.
func (z *Zone) RegisterLease(hostname string, ip net.IP, addPTR bool) {
	if hostname == "" || ip == nil {
		return
	}

	fqdn := z.fqdn(hostname)
	ttl := z.ttl

	// A record
	a := &dns.A{
		Hdr: dns.RR_Header{Name: fqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
		A:   ip.To4(),
	}
	z.Add(a)

	// PTR record
	if addPTR {
		ptrName := reverseIP(ip) + ".in-addr.arpa."
		ptr := &dns.PTR{
			Hdr: dns.RR_Header{Name: ptrName, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl},
			Ptr: fqdn,
		}
		z.Add(ptr)
	}
}

// UnregisterLease removes A and PTR records for a DHCP lease.
func (z *Zone) UnregisterLease(hostname string, ip net.IP) {
	if hostname == "" && ip == nil {
		return
	}

	if hostname != "" {
		fqdn := z.fqdn(hostname)
		z.Remove(fqdn, dns.TypeA)
	}

	if ip != nil {
		ptrName := reverseIP(ip) + ".in-addr.arpa."
		z.Remove(ptrName, dns.TypePTR)
	}
}

// fqdn builds a fully qualified domain name from a hostname.
func (z *Zone) fqdn(hostname string) string {
	hostname = strings.ToLower(hostname)
	if strings.HasSuffix(hostname, ".") {
		return hostname
	}
	if z.domain != "" && z.domain != "." {
		return hostname + "." + z.domain
	}
	return hostname + "."
}

// reverseIP creates the reversed octets for a PTR record.
func reverseIP(ip net.IP) string {
	ip = ip.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", ip[3], ip[2], ip[1], ip[0])
}

// rrValue extracts the value string from an RR for comparison.
func rrValue(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return v.Target
	case *dns.PTR:
		return v.Ptr
	case *dns.TXT:
		return strings.Join(v.Txt, " ")
	case *dns.MX:
		return fmt.Sprintf("%d %s", v.Preference, v.Mx)
	case *dns.SRV:
		return fmt.Sprintf("%d %d %d %s", v.Priority, v.Weight, v.Port, v.Target)
	default:
		return rr.String()
	}
}

// ParseStaticRecord parses a config static record into a dns.RR.
func ParseStaticRecord(name, rtype, value string, ttl uint32) (dns.RR, error) {
	fqdn := dns.Fqdn(name)
	hdr := dns.RR_Header{Name: fqdn, Class: dns.ClassINET, Ttl: ttl}

	switch strings.ToUpper(rtype) {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("invalid A record value %q", value)
		}
		hdr.Rrtype = dns.TypeA
		return &dns.A{Hdr: hdr, A: ip.To4()}, nil

	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || ip.To16() == nil || ip.To4() != nil {
			return nil, fmt.Errorf("invalid AAAA record value %q", value)
		}
		hdr.Rrtype = dns.TypeAAAA
		return &dns.AAAA{Hdr: hdr, AAAA: ip.To16()}, nil

	case "CNAME":
		hdr.Rrtype = dns.TypeCNAME
		return &dns.CNAME{Hdr: hdr, Target: dns.Fqdn(value)}, nil

	case "PTR":
		hdr.Rrtype = dns.TypePTR
		return &dns.PTR{Hdr: hdr, Ptr: dns.Fqdn(value)}, nil

	case "TXT":
		hdr.Rrtype = dns.TypeTXT
		return &dns.TXT{Hdr: hdr, Txt: []string{value}}, nil

	case "MX":
		parts := strings.Fields(value)
		if len(parts) != 2 {
			return nil, fmt.Errorf("MX value must be 'priority host', got %q", value)
		}
		var pref uint16
		if _, err := fmt.Sscanf(parts[0], "%d", &pref); err != nil {
			return nil, fmt.Errorf("invalid MX priority %q: %w", parts[0], err)
		}
		hdr.Rrtype = dns.TypeMX
		return &dns.MX{Hdr: hdr, Preference: pref, Mx: dns.Fqdn(parts[1])}, nil

	case "SRV":
		parts := strings.Fields(value)
		if len(parts) != 4 {
			return nil, fmt.Errorf("SRV value must be 'priority weight port target', got %q", value)
		}
		var priority, weight, port uint16
		if _, err := fmt.Sscanf(parts[0], "%d", &priority); err != nil {
			return nil, fmt.Errorf("invalid SRV priority: %w", err)
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &weight); err != nil {
			return nil, fmt.Errorf("invalid SRV weight: %w", err)
		}
		if _, err := fmt.Sscanf(parts[2], "%d", &port); err != nil {
			return nil, fmt.Errorf("invalid SRV port: %w", err)
		}
		hdr.Rrtype = dns.TypeSRV
		return &dns.SRV{Hdr: hdr, Priority: priority, Weight: weight, Port: port, Target: dns.Fqdn(parts[3])}, nil

	default:
		return nil, fmt.Errorf("unsupported record type %q", rtype)
	}
}
