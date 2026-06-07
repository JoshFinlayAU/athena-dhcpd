package audit

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// CSVHeaders returns the CSV column headers for audit records.
var CSVHeaders = []string{
	"id", "timestamp", "event", "ip", "mac", "client_id", "hostname", "fqdn",
	"subnet", "pool", "lease_start", "lease_expiry",
	"circuit_id", "remote_id", "giaddr", "server_id", "ha_role", "reason",
}

// WriteCSV writes audit records as CSV to the given writer.
func WriteCSV(w io.Writer, records []Record) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write(CSVHeaders); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	for _, r := range records {
		row := []string{
			strconv.FormatUint(r.ID, 10),
			r.Timestamp,
			r.Event,
			r.IP,
			r.MAC,
			csvSafe(r.ClientID),
			csvSafe(r.Hostname),
			csvSafe(r.FQDN),
			r.Subnet,
			csvSafe(r.Pool),
			formatInt64(r.LeaseStart),
			formatInt64(r.LeaseExpiry),
			csvSafe(r.CircuitID),
			csvSafe(r.RemoteID),
			r.GIAddr,
			r.ServerID,
			r.HARoleAtTime,
			csvSafe(r.Reason),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("writing CSV row: %w", err)
		}
	}
	return nil
}

func formatInt64(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

// csvSafe neutralises spreadsheet formula injection. Fields that originate from
// DHCP packets (hostname, client-id, circuit-id, etc.) are attacker-controlled;
// if such a value begins with =, +, -, @, tab or CR, a spreadsheet may execute
// it as a formula when the export is opened. Prefixing a single quote defuses it.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
