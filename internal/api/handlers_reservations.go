package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/athena-dhcpd/athena-dhcpd/internal/config"
)

// reservationResponse is the JSON representation of a reservation.
type reservationResponse struct {
	ID           int      `json:"id"`
	SubnetIndex  int      `json:"subnet_index"`
	Subnet       string   `json:"subnet"`
	MAC          string   `json:"mac,omitempty"`
	Identifier   string   `json:"identifier,omitempty"`
	IP           string   `json:"ip"`
	Hostname     string   `json:"hostname,omitempty"`
	DNSServers   []string `json:"dns_servers,omitempty"`
	DDNSHostname string   `json:"ddns_hostname,omitempty"`
}

// handleListReservations returns all reservations across all subnets.
func (s *Server) handleListReservations(w http.ResponseWriter, r *http.Request) {
	result := []reservationResponse{}
	id := 0
	for si, sub := range s.cfg.Subnets {
		for _, res := range sub.Reservations {
			result = append(result, reservationResponse{
				ID:           id,
				SubnetIndex:  si,
				Subnet:       sub.Network,
				MAC:          res.MAC,
				Identifier:   res.Identifier,
				IP:           res.IP,
				Hostname:     res.Hostname,
				DNSServers:   res.DNSServers,
				DDNSHostname: res.DDNSHostname,
			})
			id++
		}
	}

	JSONResponse(w, http.StatusOK, result)
}

// reservationRequest is the JSON body for creating/updating a reservation.
type reservationRequest struct {
	SubnetIndex  int      `json:"subnet_index"`
	MAC          string   `json:"mac,omitempty"`
	Identifier   string   `json:"identifier,omitempty"`
	IP           string   `json:"ip"`
	Hostname     string   `json:"hostname,omitempty"`
	DNSServers   []string `json:"dns_servers,omitempty"`
	DDNSHostname string   `json:"ddns_hostname,omitempty"`
}

// handleCreateReservation adds a new reservation to the config.
func (s *Server) handleCreateReservation(w http.ResponseWriter, r *http.Request) {
	var req reservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}

	if err := validateReservationRequest(s.cfg, req); err != nil {
		JSONError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	res := config.ReservationConfig{
		MAC:          req.MAC,
		Identifier:   req.Identifier,
		IP:           req.IP,
		Hostname:     req.Hostname,
		DNSServers:   req.DNSServers,
		DDNSHostname: req.DDNSHostname,
	}

	network := s.cfg.Subnets[req.SubnetIndex].Network
	if s.cfgStore != nil {
		if err := s.cfgStore.PutReservation(network, res); err != nil {
			JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
	} else {
		s.cfg.Subnets[req.SubnetIndex].Reservations = append(
			s.cfg.Subnets[req.SubnetIndex].Reservations, res,
		)
	}

	JSONResponse(w, http.StatusCreated, map[string]string{"status": "created"})
}

// handleUpdateReservation updates an existing reservation by global ID.
func (s *Server) handleUpdateReservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	targetID, err := strconv.Atoi(idStr)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_id", "reservation ID must be an integer")
		return
	}

	var req reservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}

	// Find the reservation by global ID
	id := 0
	for si := range s.cfg.Subnets {
		for ri := range s.cfg.Subnets[si].Reservations {
			if id == targetID {
				res := s.cfg.Subnets[si].Reservations[ri]
				if req.MAC != "" {
					res.MAC = req.MAC
				}
				if req.Identifier != "" {
					res.Identifier = req.Identifier
				}
				if req.IP != "" {
					ip := net.ParseIP(req.IP)
					if ip == nil {
						JSONError(w, http.StatusBadRequest, "invalid_ip", "invalid IP address")
						return
					}
					res.IP = req.IP
				}
				if req.Hostname != "" {
					res.Hostname = req.Hostname
				}
				if req.DNSServers != nil {
					res.DNSServers = req.DNSServers
				}
				if req.DDNSHostname != "" {
					res.DDNSHostname = req.DDNSHostname
				}
				network := s.cfg.Subnets[si].Network
				if s.cfgStore != nil {
					if err := s.cfgStore.PutReservation(network, res); err != nil {
						JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
						return
					}
				} else {
					s.cfg.Subnets[si].Reservations[ri] = res
				}
				JSONResponse(w, http.StatusOK, map[string]string{"status": "updated"})
				return
			}
			id++
		}
	}

	JSONError(w, http.StatusNotFound, "not_found", "reservation not found")
}

// handleDeleteReservation removes a reservation by global ID.
func (s *Server) handleDeleteReservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	targetID, err := strconv.Atoi(idStr)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_id", "reservation ID must be an integer")
		return
	}

	id := 0
	for si := range s.cfg.Subnets {
		for ri := range s.cfg.Subnets[si].Reservations {
			if id == targetID {
				mac := s.cfg.Subnets[si].Reservations[ri].MAC
				network := s.cfg.Subnets[si].Network
				if s.cfgStore != nil {
					if err := s.cfgStore.DeleteReservation(network, mac); err != nil {
						JSONError(w, http.StatusInternalServerError, "store_error", err.Error())
						return
					}
				} else {
					s.cfg.Subnets[si].Reservations = append(
						s.cfg.Subnets[si].Reservations[:ri],
						s.cfg.Subnets[si].Reservations[ri+1:]...,
					)
				}
				JSONResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
				return
			}
			id++
		}
	}

	JSONError(w, http.StatusNotFound, "not_found", "reservation not found")
}

// handleImportReservations imports reservations from CSV.
// CSV format: subnet_index,mac,identifier,ip,hostname
func (s *Server) handleImportReservations(w http.ResponseWriter, r *http.Request) {
	reader := csv.NewReader(r.Body)
	defer r.Body.Close()

	// Read header
	header, err := reader.Read()
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid_csv", "failed to read CSV header")
		return
	}
	_ = header // We expect: subnet_index,mac,identifier,ip,hostname

	imported := 0
	var errors []string

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("row %d: %v", imported+1, err))
			continue
		}
		if len(record) < 4 {
			errors = append(errors, fmt.Sprintf("row %d: need at least 4 columns", imported+1))
			continue
		}

		subnetIdx, err := strconv.Atoi(record[0])
		if err != nil || subnetIdx < 0 || subnetIdx >= len(s.cfg.Subnets) {
			errors = append(errors, fmt.Sprintf("row %d: invalid subnet_index", imported+1))
			continue
		}

		ip := net.ParseIP(record[3])
		if ip == nil {
			errors = append(errors, fmt.Sprintf("row %d: invalid IP %q", imported+1, record[3]))
			continue
		}

		res := config.ReservationConfig{
			MAC:        record[1],
			Identifier: record[2],
			IP:         record[3],
		}
		if len(record) > 4 {
			res.Hostname = record[4]
		}

		network := s.cfg.Subnets[subnetIdx].Network
		if s.cfgStore != nil {
			if err := s.cfgStore.PutReservation(network, res); err != nil {
				errors = append(errors, fmt.Sprintf("row %d: store error: %v", imported+1, err))
				continue
			}
		} else {
			s.cfg.Subnets[subnetIdx].Reservations = append(
				s.cfg.Subnets[subnetIdx].Reservations, res,
			)
		}
		imported++
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{
		"imported": imported,
		"errors":   errors,
	})
}

// handleExportReservations exports all reservations as CSV.
func (s *Server) handleExportReservations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=reservations.csv")

	cw := csv.NewWriter(w)
	cw.Write([]string{"subnet_index", "subnet", "mac", "identifier", "ip", "hostname", "ddns_hostname"})

	for si, sub := range s.cfg.Subnets {
		for _, res := range sub.Reservations {
			cw.Write([]string{
				strconv.Itoa(si),
				sub.Network,
				res.MAC,
				res.Identifier,
				res.IP,
				res.Hostname,
				res.DDNSHostname,
			})
		}
	}
	cw.Flush()
}

// validateReservationRequest validates a reservation create request.
func validateReservationRequest(cfg *config.Config, req reservationRequest) error {
	if req.MAC == "" && req.Identifier == "" {
		return fmt.Errorf("mac or identifier is required")
	}
	if req.IP == "" {
		return fmt.Errorf("ip is required")
	}
	ip := net.ParseIP(req.IP)
	if ip == nil {
		return fmt.Errorf("invalid IP address %q", req.IP)
	}
	if req.SubnetIndex < 0 || req.SubnetIndex >= len(cfg.Subnets) {
		return fmt.Errorf("subnet_index %d out of range (0-%d)", req.SubnetIndex, len(cfg.Subnets)-1)
	}
	if req.MAC != "" {
		if _, err := net.ParseMAC(req.MAC); err != nil {
			return fmt.Errorf("invalid MAC address %q: %w", req.MAC, err)
		}
	}
	return nil
}
