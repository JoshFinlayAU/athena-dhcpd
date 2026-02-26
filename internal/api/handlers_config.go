package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
