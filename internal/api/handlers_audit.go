package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/audit"
)

// handleAuditQuery searches the audit log with query parameters.
// GET /api/v2/audit?ip=&mac=&event=&from=&to=&at=&limit=
func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		JSONError(w, http.StatusServiceUnavailable, "audit_disabled", "audit log not available")
		return
	}

	params := audit.QueryParams{}
	q := r.URL.Query()

	params.IP = q.Get("ip")
	params.MAC = q.Get("mac")
	params.Event = q.Get("event")

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			params.Limit = n
		}
	}

	if v := q.Get("at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.At = t
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.To = t
		}
	}

	records, err := s.auditLog.Query(params)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"count":   len(records),
		"records": records,
	})
}

// handleAuditExportCSV exports audit log records as CSV.
// GET /api/v2/audit/export?ip=&mac=&event=&from=&to=&at=&limit=
func (s *Server) handleAuditExportCSV(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		JSONError(w, http.StatusServiceUnavailable, "audit_disabled", "audit log not available")
		return
	}

	params := audit.QueryParams{}
	q := r.URL.Query()

	params.IP = q.Get("ip")
	params.MAC = q.Get("mac")
	params.Event = q.Get("event")

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			params.Limit = n
		}
	}
	if v := q.Get("at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.At = t
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.To = t
		}
	}

	records, err := s.auditLog.Query(params)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_log.csv")
	if err := audit.WriteCSV(w, records); err != nil {
		s.logger.Error("failed to write CSV export", "error", err)
	}
}

// handleAuditStats returns summary statistics about the audit log.
// GET /api/v2/audit/stats
func (s *Server) handleAuditStats(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		JSONError(w, http.StatusServiceUnavailable, "audit_disabled", "audit log not available")
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"total_records": s.auditLog.Count(),
	})
}
