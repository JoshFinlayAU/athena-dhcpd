// Package lease manages DHCP lease lifecycle, storage, and garbage collection.
package lease

import (
	"encoding/json"
	"net"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Lease represents a DHCP lease.
type Lease struct {
	IP          net.IP              `json:"ip"`
	MAC         net.HardwareAddr    `json:"mac"`
	ClientID    string              `json:"client_id,omitempty"`
	Hostname    string              `json:"hostname,omitempty"`
	FQDN        string              `json:"fqdn,omitempty"`
	Subnet      string              `json:"subnet"`
	Pool        string              `json:"pool,omitempty"`
	State       dhcpv4.LeaseState   `json:"state"`
	Start       time.Time           `json:"start"`
	Expiry      time.Time           `json:"expiry"`
	LastUpdated time.Time           `json:"last_updated"`
	UpdateSeq   uint64              `json:"update_seq"`
	Options     map[string]string   `json:"options,omitempty"`
	RelayInfo   *RelayInfo          `json:"relay_info,omitempty"`
}

// RelayInfo stores relay agent information associated with a lease.
type RelayInfo struct {
	GIAddr    net.IP `json:"giaddr,omitempty"`
	CircuitID string `json:"circuit_id,omitempty"`
	RemoteID  string `json:"remote_id,omitempty"`
}

// IsExpired returns true if the lease has expired.
func (l *Lease) IsExpired() bool {
	return time.Now().After(l.Expiry)
}

// Remaining returns the time remaining on the lease.
func (l *Lease) Remaining() time.Duration {
	r := time.Until(l.Expiry)
	if r < 0 {
		return 0
	}
	return r
}

// Duration returns the total lease duration.
func (l *Lease) Duration() time.Duration {
	return l.Expiry.Sub(l.Start)
}

// MarshalJSON implements custom JSON marshalling.
func (l *Lease) MarshalJSON() ([]byte, error) {
	type Alias Lease
	return json.Marshal(&struct {
		IP  string `json:"ip"`
		MAC string `json:"mac"`
		*Alias
	}{
		IP:    l.IP.String(),
		MAC:   l.MAC.String(),
		Alias: (*Alias)(l),
	})
}

// UnmarshalJSON implements custom JSON unmarshalling.
func (l *Lease) UnmarshalJSON(data []byte) error {
	type Alias Lease
	aux := &struct {
		IP  string `json:"ip"`
		MAC string `json:"mac"`
		*Alias
	}{
		Alias: (*Alias)(l),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	l.IP = net.ParseIP(aux.IP)
	var err error
	l.MAC, err = net.ParseMAC(aux.MAC)
	if err != nil {
		return err
	}
	return nil
}

// Key returns the BoltDB key for this lease (IP string).
func (l *Lease) Key() string {
	return l.IP.String()
}

// Clone returns a deep copy of the lease.
func (l *Lease) Clone() *Lease {
	c := *l
	c.IP = make(net.IP, len(l.IP))
	copy(c.IP, l.IP)
	c.MAC = make(net.HardwareAddr, len(l.MAC))
	copy(c.MAC, l.MAC)
	if l.Options != nil {
		c.Options = make(map[string]string, len(l.Options))
		for k, v := range l.Options {
			c.Options[k] = v
		}
	}
	if l.RelayInfo != nil {
		ri := *l.RelayInfo
		c.RelayInfo = &ri
	}
	return &c
}
