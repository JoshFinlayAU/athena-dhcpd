package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// WriteHASection updates only the [ha] section of a TOML config file,
// preserving all other sections. Creates a timestamped backup before writing.
// The write is atomic (temp file + rename).
func WriteHASection(path string, ha *HAConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	// Build the new [ha] block
	var buf bytes.Buffer
	buf.WriteString("[ha]\n")
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(haToMap(ha)); err != nil {
		return fmt.Errorf("encoding HA config: %w", err)
	}

	content := string(data)
	newHA := buf.String()

	// Replace existing [ha] section using line-based parsing.
	// Go's regexp package does not support Perl lookahead (?=...) so we
	// walk lines instead: skip everything from [ha] (or [ha.*]) until the
	// next top-level section that isn't a sub-table of ha.
	content = replaceOrAppendSection(content, "ha", newHA)

	// Create backup (best-effort: try config dir, fall back to temp dir)
	ts := time.Now().Format("20060102T150405")
	backupPath := path + ".bak." + ts
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		// Fall back to temp directory
		tmpBackup := filepath.Join(os.TempDir(), filepath.Base(path)+".bak."+ts)
		if err2 := os.WriteFile(tmpBackup, data, 0600); err2 != nil {
			// Non-fatal: proceed without backup rather than blocking config writes
			fmt.Fprintf(os.Stderr, "warning: could not create config backup (tried %s and %s): %v\n", backupPath, tmpBackup, err)
		}
	}

	// Atomic write: temp file + rename
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "athena-config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming config: %w", err)
	}

	return nil
}

// replaceOrAppendSection replaces a TOML top-level section (and its sub-tables)
// with newContent, or appends it if the section doesn't exist.
// sectionName should be bare, e.g. "ha" — it matches [ha] and [ha.*].
func replaceOrAppendSection(content, sectionName, newContent string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false
	replaced := false
	header := "[" + sectionName + "]"
	subPrefix := "[" + sectionName + "."

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inSection {
			if trimmed == header || strings.HasPrefix(trimmed, subPrefix) {
				inSection = true
				if !replaced {
					result = append(result, strings.TrimRight(newContent, "\n"))
					replaced = true
				}
				continue
			}
			result = append(result, line)
		} else {
			// Check if this line starts a new section that isn't part of our target
			if len(trimmed) > 0 && trimmed[0] == '[' && trimmed != header && !strings.HasPrefix(trimmed, subPrefix) {
				inSection = false
				result = append(result, line)
				continue
			}
			// Still inside the target section — skip
			continue
		}
	}

	if !replaced {
		return strings.TrimRight(strings.Join(result, "\n"), "\n") + "\n\n" + newContent
	}
	return strings.Join(result, "\n")
}

// haToMap converts HAConfig to a flat map for TOML encoding without the
// wrapping [ha] table header (we add that ourselves).
func haToMap(ha *HAConfig) map[string]interface{} {
	m := map[string]interface{}{
		"enabled": ha.Enabled,
	}
	if ha.Role != "" {
		m["role"] = ha.Role
	}
	if ha.PeerAddress != "" {
		m["peer_address"] = ha.PeerAddress
	}
	if ha.ListenAddress != "" {
		m["listen_address"] = ha.ListenAddress
	}
	if ha.HeartbeatInterval != "" {
		m["heartbeat_interval"] = ha.HeartbeatInterval
	}
	if ha.FailoverTimeout != "" {
		m["failover_timeout"] = ha.FailoverTimeout
	}
	if ha.SyncBatchSize > 0 {
		m["sync_batch_size"] = ha.SyncBatchSize
	}
	if ha.TLS.Enabled || ha.TLS.CertFile != "" || ha.TLS.KeyFile != "" || ha.TLS.CAFile != "" {
		tls := map[string]interface{}{
			"enabled": ha.TLS.Enabled,
		}
		if ha.TLS.CertFile != "" {
			tls["cert_file"] = ha.TLS.CertFile
		}
		if ha.TLS.KeyFile != "" {
			tls["key_file"] = ha.TLS.KeyFile
		}
		if ha.TLS.CAFile != "" {
			tls["ca_file"] = ha.TLS.CAFile
		}
		m["tls"] = tls
	}
	return m
}
