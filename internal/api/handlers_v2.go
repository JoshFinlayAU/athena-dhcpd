package api

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// --- Subnets ---

func (s *Server) handleV2ListSubnets(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.Subnets())
}

func (s *Server) handleV2CreateSubnet(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var sub config.SubnetConfig
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if sub.Network == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "network is required")
		return
	}
	if _, exists := s.cfgStore.GetSubnet(sub.Network); exists {
		JSONError(w, http.StatusConflict, "already_exists", "subnet already exists")
		return
	}
	if err := s.cfgStore.PutSubnet(sub); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusCreated, sub)
}

func (s *Server) handleV2UpdateSubnet(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))
	existing, exists := s.cfgStore.GetSubnet(network)
	if !exists {
		JSONError(w, http.StatusNotFound, "not_found", "subnet not found")
		return
	}
	var sub config.SubnetConfig
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	sub.Network = network
	// Preserve pools, reservations, and options if not sent in the update
	if sub.Pools == nil {
		sub.Pools = existing.Pools
	}
	if sub.Reservations == nil {
		sub.Reservations = existing.Reservations
	}
	if sub.Options == nil {
		sub.Options = existing.Options
	}
	if err := s.cfgStore.PutSubnet(sub); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, sub)
}

func (s *Server) handleV2DeleteSubnet(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))
	if err := s.cfgStore.DeleteSubnet(network); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Reservations ---

func (s *Server) handleV2ListReservations(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))
	res := s.cfgStore.GetReservations(network)
	if res == nil {
		res = []config.ReservationConfig{}
	}
	JSONResponse(w, http.StatusOK, res)
}

func (s *Server) handleV2CreateReservation(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))
	var res config.ReservationConfig
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if res.MAC == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "mac is required")
		return
	}
	if res.IP == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "ip is required")
		return
	}
	if err := s.cfgStore.PutReservation(network, res); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusCreated, res)
}

func (s *Server) handleV2DeleteReservation(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))
	mac, _ := url.PathUnescape(r.PathValue("mac"))
	if err := s.cfgStore.DeleteReservation(network, mac); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleV2ImportReservations(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	network, _ := url.PathUnescape(r.PathValue("network"))

	contentType := r.Header.Get("Content-Type")
	var reservations []config.ReservationConfig

	if strings.Contains(contentType, "text/csv") || strings.Contains(contentType, "multipart/form-data") {
		// CSV import
		var reader io.Reader
		if strings.Contains(contentType, "multipart/form-data") {
			file, _, err := r.FormFile("file")
			if err != nil {
				JSONError(w, http.StatusBadRequest, "no_file", "file upload required")
				return
			}
			defer file.Close()
			reader = file
		} else {
			reader = r.Body
		}

		csvReader := csv.NewReader(reader)
		records, err := csvReader.ReadAll()
		if err != nil {
			JSONError(w, http.StatusBadRequest, "csv_error", err.Error())
			return
		}

		// Parse CSV: expect mac,ip,hostname (header optional)
		for i, record := range records {
			if len(record) < 2 {
				continue
			}
			// Skip header row
			if i == 0 && (strings.EqualFold(record[0], "mac") || strings.EqualFold(record[0], "MAC")) {
				continue
			}
			res := config.ReservationConfig{
				MAC: strings.TrimSpace(record[0]),
				IP:  strings.TrimSpace(record[1]),
			}
			if len(record) > 2 {
				res.Hostname = strings.TrimSpace(record[2])
			}
			if len(record) > 3 {
				res.DDNSHostname = strings.TrimSpace(record[3])
			}
			if res.MAC != "" && res.IP != "" {
				reservations = append(reservations, res)
			}
		}
	} else {
		// JSON import
		if err := json.NewDecoder(r.Body).Decode(&reservations); err != nil {
			JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}

	if len(reservations) == 0 {
		JSONError(w, http.StatusBadRequest, "empty", "no reservations to import")
		return
	}

	added, err := s.cfgStore.ImportReservations(network, reservations)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "import_error", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"imported": len(reservations),
		"added":    added,
		"updated":  len(reservations) - added,
	})
}

// --- Singleton config sections ---

func (s *Server) handleV2GetDefaults(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.Defaults())
}

func (s *Server) handleV2SetDefaults(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var d config.DefaultsConfig
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.cfgStore.SetDefaults(d); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, d)
}

func (s *Server) handleV2GetConflict(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.ConflictDetection())
}

func (s *Server) handleV2SetConflict(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var c config.ConflictDetectionConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.cfgStore.SetConflictDetection(c); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, c)
}

func (s *Server) handleV2GetHA(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, http.StatusOK, s.cfg.HA)
}

func (s *Server) handleV2SetHA(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		JSONError(w, http.StatusBadRequest, "no_config_path", "config file path not set")
		return
	}
	var h config.HAConfig
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := config.WriteHASection(s.configPath, &h); err != nil {
		JSONError(w, http.StatusInternalServerError, "write_error", err.Error())
		return
	}
	// Update in-memory config immediately
	s.cfg.HA = h
	s.logger.Info("HA config updated via API", "role", h.Role, "enabled", h.Enabled)
	JSONResponse(w, http.StatusOK, h)
}

func (s *Server) handleV2GetHooks(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.Hooks())
}

func (s *Server) handleV2SetHooks(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var h config.HooksConfig
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.cfgStore.SetHooks(h); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, h)
}

func (s *Server) handleV2GetDDNS(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.DDNS())
}

func (s *Server) handleV2SetDDNS(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var d config.DDNSConfig
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.cfgStore.SetDDNS(d); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, d)
}

func (s *Server) handleV2GetDNS(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	JSONResponse(w, http.StatusOK, s.cfgStore.DNS())
}

func (s *Server) handleV2SetDNS(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}
	var d config.DNSProxyConfig
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.cfgStore.SetDNS(d); err != nil {
		JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	JSONResponse(w, http.StatusOK, d)
}

// --- V1 TOML Import ---

func (s *Server) handleV2ImportTOML(w http.ResponseWriter, r *http.Request) {
	if s.cfgStore == nil {
		JSONError(w, http.StatusServiceUnavailable, "no_config_store", "config store not available")
		return
	}

	var body struct {
		TOML string `json:"toml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.TOML == "" {
		JSONError(w, http.StatusBadRequest, "missing_field", "toml field is required")
		return
	}

	var cfg config.Config
	if err := toml.Unmarshal([]byte(body.TOML), &cfg); err != nil {
		JSONError(w, http.StatusBadRequest, "parse_error", err.Error())
		return
	}

	if err := s.cfgStore.ImportFromConfig(&cfg); err != nil {
		JSONError(w, http.StatusInternalServerError, "import_error", err.Error())
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "imported",
		"subnets": len(cfg.Subnets),
	})
}
