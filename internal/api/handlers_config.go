package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// handleGetConfigRaw returns the running config as raw TOML text.
func (s *Server) handleGetConfigRaw(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		JSONError(w, http.StatusNotFound, "no_config_path", "config file path not set")
		return
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "read_failed", fmt.Sprintf("reading config: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleValidateConfig validates a config without applying it.
func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_body", "expected JSON with 'config' field containing TOML")
		return
	}

	var cfg config.Config
	if _, err := toml.Decode(body.Config, &cfg); err != nil {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"valid":  false,
			"errors": []string{fmt.Sprintf("TOML parse error: %v", err)},
		})
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"valid":  true,
		"errors": []string{},
	})
}

// handleUpdateConfig validates, backs up, and writes a new config.
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		JSONError(w, http.StatusBadRequest, "no_config_path", "config file path not set")
		return
	}

	var body struct {
		Config string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_body", "expected JSON with 'config' field containing TOML")
		return
	}

	// Validate
	var cfg config.Config
	if _, err := toml.Decode(body.Config, &cfg); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_config", fmt.Sprintf("TOML parse error: %v", err))
		return
	}

	// Create backup
	backupPath, err := s.backupConfig()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "backup_failed", err.Error())
		return
	}

	// Atomic write: write to temp file, then rename
	dir := filepath.Dir(s.configPath)
	tmpFile, err := os.CreateTemp(dir, "athena-config-*.toml.tmp")
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "write_failed", fmt.Sprintf("creating temp file: %v", err))
		return
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(body.Config); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		JSONError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	tmpFile.Close()

	if err := os.Rename(tmpPath, s.configPath); err != nil {
		os.Remove(tmpPath)
		JSONError(w, http.StatusInternalServerError, "write_failed", fmt.Sprintf("renaming config: %v", err))
		return
	}

	s.logger.Info("config updated via API", "backup", backupPath)

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"backup": filepath.Base(backupPath),
	})
}

// handleListConfigBackups returns available config backups.
func (s *Server) handleListConfigBackups(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		JSONResponse(w, http.StatusOK, []interface{}{})
		return
	}

	dir := filepath.Dir(s.configPath)
	base := filepath.Base(s.configPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}

	type backupEntry struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Modified string `json:"modified"`
	}

	var backups []backupEntry
	prefix := base + ".bak."

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			backups = append(backups, backupEntry{
				Name:     entry.Name(),
				Size:     info.Size(),
				Modified: info.ModTime().Format(time.RFC3339),
			})
		}
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Modified > backups[j].Modified
	})

	JSONResponse(w, http.StatusOK, backups)
}

// handleGetConfigBackup downloads a specific config backup.
func (s *Server) handleGetConfigBackup(w http.ResponseWriter, r *http.Request) {
	ts := r.PathValue("timestamp")
	if ts == "" {
		JSONError(w, http.StatusBadRequest, "missing_timestamp", "backup timestamp required")
		return
	}

	if s.configPath == "" {
		JSONError(w, http.StatusNotFound, "no_config_path", "config file path not set")
		return
	}

	dir := filepath.Dir(s.configPath)
	base := filepath.Base(s.configPath)
	backupName := base + ".bak." + ts

	// Sanitize to prevent path traversal
	if strings.Contains(ts, "/") || strings.Contains(ts, "..") {
		JSONError(w, http.StatusBadRequest, "invalid_timestamp", "invalid timestamp format")
		return
	}

	backupPath := filepath.Join(dir, backupName)
	data, err := os.ReadFile(backupPath)
	if err != nil {
		JSONError(w, http.StatusNotFound, "not_found", "backup not found")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backupName))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// backupConfig creates a timestamped backup of the current config file.
func (s *Server) backupConfig() (string, error) {
	if s.configPath == "" {
		return "", fmt.Errorf("config path not set")
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return "", fmt.Errorf("reading config for backup: %w", err)
	}

	ts := time.Now().Format("20060102T150405")
	backupPath := s.configPath + ".bak." + ts

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("writing backup: %w", err)
	}

	return backupPath, nil
}
