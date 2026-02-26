package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/athena-dhcpd/athena-dhcpd/internal/dnsproxy"
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

// handleDNSListStatus returns the status of all DNS filter lists.
func (s *Server) handleDNSListStatus(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	lists := s.dns.Lists()
	if lists == nil {
		JSONResponse(w, http.StatusOK, map[string]interface{}{"lists": []interface{}{}})
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"lists":         lists.Statuses(),
		"total_domains": lists.TotalDomains(),
	})
}

// handleDNSListRefresh triggers a manual refresh of DNS filter lists.
func (s *Server) handleDNSListRefresh(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	lists := s.dns.Lists()
	if lists == nil {
		JSONError(w, http.StatusNotFound, "no_lists", "No DNS filter lists configured")
		return
	}

	// Check if a specific list name is provided
	var body struct {
		Name string `json:"name"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}

	if body.Name != "" {
		if err := lists.RefreshByName(body.Name); err != nil {
			JSONError(w, http.StatusNotFound, "list_not_found", err.Error())
			return
		}
		JSONResponse(w, http.StatusOK, map[string]string{"status": "refreshed", "list": body.Name})
		return
	}

	lists.RefreshAll()
	JSONResponse(w, http.StatusOK, map[string]string{"status": "all_refreshed"})
}

// handleDNSListTest tests a domain against all active DNS filter lists.
func (s *Server) handleDNSListTest(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	lists := s.dns.Lists()
	if lists == nil {
		JSONError(w, http.StatusNotFound, "no_lists", "No DNS filter lists configured")
		return
	}

	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain == "" {
		JSONError(w, http.StatusBadRequest, "bad_request", "Provide {\"domain\": \"example.com\"}")
		return
	}

	result := lists.TestDomain(body.Domain)
	JSONResponse(w, http.StatusOK, result)
}

// handleDNSQueryLog returns recent DNS query log entries.
func (s *Server) handleDNSQueryLog(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	entries := s.dns.GetQueryLog().Recent(limit)
	if entries == nil {
		entries = []dnsproxy.QueryLogEntry{}
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   s.dns.GetQueryLog().Count(),
	})
}

// handleDNSQueryLogStream streams DNS query log entries via SSE.
func (s *Server) handleDNSQueryLogStream(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		JSONError(w, http.StatusServiceUnavailable, "dns_disabled", "DNS proxy is not enabled")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	qlog := s.dns.GetQueryLog()
	subID, ch := qlog.Subscribe(256)
	defer qlog.Unsubscribe(subID)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
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
