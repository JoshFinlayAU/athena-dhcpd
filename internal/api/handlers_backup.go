package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// backupPayload is the JSON structure for backup export/import.
type backupPayload struct {
	Version   string                     `json:"version"`
	Timestamp string                     `json:"timestamp"`
	ServerID  string                     `json:"server_id,omitempty"`
	Config    map[string]json.RawMessage `json:"config"`
	Leases    []json.RawMessage          `json:"leases,omitempty"`
	Users     []backupUser               `json:"users,omitempty"`
}

type backupUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
}

// handleBackupExport exports all config, leases, and users as a single JSON file.
// GET /api/v2/backup
func (s *Server) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	backup := backupPayload{
		Version:   "1",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ServerID:  s.cfg.Server.ServerID,
		Config:    make(map[string]json.RawMessage),
	}

	// Export config sections from dbconfig store
	if s.cfgStore != nil {
		sections := s.cfgStore.ExportAllSections()
		for k, v := range sections {
			backup.Config[k] = json.RawMessage(v)
		}
	}

	// Export HA config from in-memory config (lives in TOML, not DB)
	if haData, err := json.Marshal(s.cfg.HA); err == nil {
		backup.Config["ha"] = json.RawMessage(haData)
	}

	// Export leases
	if s.leaseStore != nil {
		leases := s.leaseStore.All()
		for _, l := range leases {
			if data, err := json.Marshal(l); err == nil {
				backup.Leases = append(backup.Leases, json.RawMessage(data))
			}
		}
	}

	// Export users (with hashes — this is an admin-only backup)
	if s.cfgStore != nil {
		users := s.cfgStore.Users()
		for _, u := range users {
			backup.Users = append(backup.Users, backupUser{
				Username:     u.Username,
				PasswordHash: u.PasswordHash,
				Role:         u.Role,
			})
		}
	}

	filename := fmt.Sprintf("athena-backup-%s.json", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	json.NewEncoder(w).Encode(backup)
}

// handleBackupRestore imports config and users from a backup JSON file.
// POST /api/v2/backup/restore
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	var backup backupPayload
	if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
		JSONError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid backup file: %s", err.Error()))
		return
	}

	if backup.Version == "" {
		JSONError(w, http.StatusBadRequest, "bad_request", "missing backup version field")
		return
	}

	restored := make([]string, 0)

	// Restore config sections via ApplyPeerConfig (same mechanism as HA sync)
	for section, data := range backup.Config {
		// Skip HA — that's node-identity config, don't overwrite from backup
		if section == "ha" {
			continue
		}
		if err := s.cfgStore.ApplyPeerConfig(section, data); err != nil {
			s.logger.Warn("failed to restore config section", "section", section, "error", err)
		} else {
			restored = append(restored, section)
		}
	}

	// Restore users
	usersRestored := 0
	for _, u := range backup.Users {
		cfg := config.UserConfig{
			Username:     u.Username,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
		}
		if err := s.cfgStore.PutUser(cfg); err != nil {
			s.logger.Warn("failed to restore user", "username", u.Username, "error", err)
		} else {
			usersRestored++
		}
	}
	if usersRestored > 0 {
		s.auth.UpdateUsers(s.cfgStore.Users())
		restored = append(restored, fmt.Sprintf("users (%d)", usersRestored))
	}

	s.logger.Info("backup restore complete", "sections", restored)
	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":   "restored",
		"sections": restored,
	})
}
