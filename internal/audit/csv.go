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
			r.ClientID,
			r.Hostname,
			r.FQDN,
			r.Subnet,
			r.Pool,
			formatInt64(r.LeaseStart),
			formatInt64(r.LeaseExpiry),
			r.CircuitID,
			r.RemoteID,
			r.GIAddr,
			r.ServerID,
			r.HARoleAtTime,
			r.Reason,
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
