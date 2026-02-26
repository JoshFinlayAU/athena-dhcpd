package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// --- Setup Wizard API ---

// handleSetupStatus returns the current setup state.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONResponse(w, http.StatusOK, map[string]interface{}{
			"needs_setup": true,
		})
		return
	}
	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"needs_setup": !s.cfgStore.IsSetupComplete(),
	})
}

// setupHARequest is the JSON body for the HA setup step.
type setupHARequest struct {
	Mode          string `json:"mode"`           // "standalone" or "ha"
	Role          string `json:"role"`           // "primary" or "secondary" (only if mode=ha)
	PeerAddress   string `json:"peer_address"`   // e.g. "10.0.0.2:8068"
	ListenAddress string `json:"listen_address"` // e.g. "0.0.0.0:8068"
	TLSEnabled    bool   `json:"tls_enabled"`
	TLSCA         string `json:"tls_ca"`   // PEM text (optional)
	TLSCert       string `json:"tls_cert"` // PEM text (optional)
	TLSKey        string `json:"tls_key"`  // PEM text (optional)
}

// handleSetupHA saves HA configuration during the setup wizard.
func (s *Server) handleSetupHA(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		JSONError(w, http.StatusBadRequest, "no_config_path", "config file path not set")
		return
	}

	var req setupHARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if req.Mode == "standalone" {
		// Standalone mode — write HA as disabled to TOML
		if err := config.WriteHASection(s.configPath, &config.HAConfig{Enabled: false}); err != nil {
			JSONError(w, http.StatusInternalServerError, "write_error", err.Error())
			return
		}
		s.cfg.HA = config.HAConfig{Enabled: false}
		JSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "mode": "standalone"})
		return
	}

	if req.Mode != "ha" {
		JSONError(w, http.StatusBadRequest, "invalid_mode", "mode must be 'standalone' or 'ha'")
		return
	}

	if req.Role != "primary" && req.Role != "secondary" {
		JSONError(w, http.StatusBadRequest, "invalid_role", "role must be 'primary' or 'secondary'")
		return
	}
	if req.PeerAddress == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "peer_address is required for HA")
		return
	}
	if req.ListenAddress == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "listen_address is required for HA")
		return
	}

	haCfg := config.HAConfig{
		Enabled:       true,
		Role:          req.Role,
		PeerAddress:   req.PeerAddress,
		ListenAddress: req.ListenAddress,
	}

	// Write TLS certs to disk if provided
	if req.TLSEnabled {
		tlsDir := s.tlsCertDir()
		if err := os.MkdirAll(tlsDir, 0700); err != nil {
			JSONError(w, http.StatusInternalServerError, "tls_error",
				fmt.Sprintf("creating TLS directory: %v", err))
			return
		}

		haCfg.TLS.Enabled = true

		if req.TLSCA != "" {
			caPath := filepath.Join(tlsDir, "ha-ca.pem")
			if err := os.WriteFile(caPath, []byte(req.TLSCA), 0600); err != nil {
				JSONError(w, http.StatusInternalServerError, "tls_error",
					fmt.Sprintf("writing CA cert: %v", err))
				return
			}
			haCfg.TLS.CAFile = caPath
		}
		if req.TLSCert != "" {
			certPath := filepath.Join(tlsDir, "ha-cert.pem")
			if err := os.WriteFile(certPath, []byte(req.TLSCert), 0600); err != nil {
				JSONError(w, http.StatusInternalServerError, "tls_error",
					fmt.Sprintf("writing cert: %v", err))
				return
			}
			haCfg.TLS.CertFile = certPath
		}
		if req.TLSKey != "" {
			keyPath := filepath.Join(tlsDir, "ha-key.pem")
			if err := os.WriteFile(keyPath, []byte(req.TLSKey), 0600); err != nil {
				JSONError(w, http.StatusInternalServerError, "tls_error",
					fmt.Sprintf("writing key: %v", err))
				return
			}
			haCfg.TLS.KeyFile = keyPath
		}
	}

	// Write HA config to TOML file (HA lives in TOML, not the database)
	if err := config.WriteHASection(s.configPath, &haCfg); err != nil {
		JSONError(w, http.StatusInternalServerError, "write_error", err.Error())
		return
	}
	s.cfg.HA = haCfg

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"mode":   "ha",
		"role":   req.Role,
	})
}

// setupConfigRequest is the JSON body for the config setup step.
type setupConfigRequest struct {
	Defaults *config.DefaultsConfig          `json:"defaults,omitempty"`
	Subnets  []config.SubnetConfig           `json:"subnets,omitempty"`
	Conflict *config.ConflictDetectionConfig `json:"conflict_detection,omitempty"`
	DNS      *config.DNSProxyConfig          `json:"dns,omitempty"`
	DDNS     *config.DDNSConfig              `json:"ddns,omitempty"`
}

// handleSetupConfig saves DHCP/DNS configuration during the setup wizard.
func (s *Server) handleSetupConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	var req setupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if req.Defaults != nil {
		if err := s.cfgStore.SetDefaults(*req.Defaults); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error",
				fmt.Sprintf("saving defaults: %v", err))
			return
		}
	}

	for _, sub := range req.Subnets {
		if sub.Network == "" {
			continue
		}
		if err := s.cfgStore.PutSubnet(sub); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error",
				fmt.Sprintf("saving subnet %s: %v", sub.Network, err))
			return
		}
	}

	if req.Conflict != nil {
		if err := s.cfgStore.SetConflictDetection(*req.Conflict); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error",
				fmt.Sprintf("saving conflict detection: %v", err))
			return
		}
	}

	if req.DNS != nil {
		if err := s.cfgStore.SetDNS(*req.DNS); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error",
				fmt.Sprintf("saving DNS config: %v", err))
			return
		}
	}

	if req.DDNS != nil {
		if err := s.cfgStore.SetDDNS(*req.DDNS); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error",
				fmt.Sprintf("saving DDNS config: %v", err))
			return
		}
	}

	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSetupComplete marks setup as done and triggers full service startup.
func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	if err := s.cfgStore.MarkSetupComplete(); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Fire the setup complete callback to start all services
	if s.onSetupComplete != nil {
		go s.onSetupComplete()
	}

	JSONResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "setup complete — services starting",
	})
}

// tlsCertDir returns the directory to store TLS certs (next to the lease DB).
func (s *Server) tlsCertDir() string {
	if s.cfg.Server.LeaseDB != "" {
		return filepath.Join(filepath.Dir(s.cfg.Server.LeaseDB), "tls")
	}
	return "/var/lib/athena-dhcpd/tls"
}
