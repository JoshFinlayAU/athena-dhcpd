package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/miekg/dns"
)

// handleDNSStats returns DNS proxy statistics.
func (s *Server) handleDNSStats(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	JSONResponse(w, http.StatusOK, s.dns.Stats())
}

// handleDNSFlushCache clears the DNS response cache.
func (s *Server) handleDNSFlushCache(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	s.dns.FlushCache()
	JSONResponse(w, http.StatusOK, map[string]string{"status": "flushed"})
}

// handleDNSListRecords returns all records in the local DNS zone.
func (s *Server) handleDNSListRecords(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	zone := s.dns.Zone()
	records := zone.AllRecords()

	type recordResponse struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value string `json:"value"`
		TTL   uint32 `json:"ttl"`
	}

	var result []recordResponse
	for _, rr := range records {
		result = append(result, recordResponse{
			Name:  rr.Header().Name,
			Type:  dnsTypeString(rr.Header().Rrtype),
			Value: rrValueString(rr),
			TTL:   rr.Header().Ttl,
		})
	}

	if result == nil {
		result = []recordResponse{}
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"records": result,
		"count":   len(result),
	})
}

// dnsTypeString converts a DNS type uint16 to a human-readable string.
func dnsTypeString(t uint16) string {
	if s, ok := dns.TypeToString[t]; ok {
		return s
	}
	return fmt.Sprintf("TYPE%d", t)
}

// rrValueString extracts the value from a dns.RR for display.
func rrValueString(rr dns.RR) string {
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
