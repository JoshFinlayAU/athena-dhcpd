// Package events provides the event bus and hook dispatcher for athena-dhcpd.
package events

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// EventType represents a DHCP lifecycle or system event.
type EventType string

const (
	EventLeaseDiscover     EventType = "lease.discover"
	EventLeaseOffer        EventType = "lease.offer"
	EventLeaseAck          EventType = "lease.ack"
	EventLeaseRenew        EventType = "lease.renew"
	EventLeaseNak          EventType = "lease.nak"
	EventLeaseRelease      EventType = "lease.release"
	EventLeaseDecline      EventType = "lease.decline"
	EventLeaseExpire       EventType = "lease.expire"
	EventConflictDetected  EventType = "conflict.detected"
	EventConflictDecline   EventType = "conflict.decline"
	EventConflictResolved  EventType = "conflict.resolved"
	EventConflictPermanent EventType = "conflict.permanent"
	EventHAFailover        EventType = "ha.failover"
	EventHASyncComplete    EventType = "ha.sync_complete"
	EventRogueDetected     EventType = "rogue.detected"
	EventRogueResolved     EventType = "rogue.resolved"
	EventAnomalyDetected   EventType = "anomaly.detected"
)

// Event is the core event payload passed through the event bus.
type Event struct {
	Type      EventType     `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Lease     *LeaseData    `json:"lease,omitempty"`
	Conflict  *ConflictData `json:"conflict,omitempty"`
	Server    *ServerData   `json:"server,omitempty"`
	HA        *HAData       `json:"ha,omitempty"`
	Rogue     *RogueData    `json:"rogue,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

// LeaseData carries lease information in events.
type LeaseData struct {
	IP       net.IP                 `json:"ip"`
	MAC      net.HardwareAddr       `json:"mac"`
	ClientID string                 `json:"client_id,omitempty"`
	Hostname string                 `json:"hostname,omitempty"`
	FQDN     string                 `json:"fqdn,omitempty"`
	Subnet   string                 `json:"subnet"`
	Pool     string                 `json:"pool,omitempty"`
	Start    int64                  `json:"start"`
	Expiry   int64                  `json:"expiry"`
	State    string                 `json:"state"`
	OldIP    net.IP                 `json:"old_ip,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
	Relay    *RelayData             `json:"relay,omitempty"`
}

// RelayData carries relay agent info in events.
type RelayData struct {
	GIAddr    net.IP `json:"giaddr,omitempty"`
	CircuitID string `json:"circuit_id,omitempty"`
	RemoteID  string `json:"remote_id,omitempty"`
}

// ConflictData carries conflict information in events.
type ConflictData struct {
	IP                net.IP `json:"ip"`
	Subnet            string `json:"subnet"`
	DetectionMethod   string `json:"detection_method"`
	ResponderMAC      string `json:"responder_mac,omitempty"`
	ProbeCount        int    `json:"probe_count"`
	HoldUntil         string `json:"hold_until,omitempty"`
	IntendedClientMAC string `json:"intended_client_mac,omitempty"`
	ResolutionMethod  string `json:"resolution_method,omitempty"`
}

// ServerData carries server identification in events.
type ServerData struct {
	NodeID string `json:"node_id"`
	HARole string `json:"ha_role"`
}

// HAData carries HA state change information in events.
type HAData struct {
	OldRole   string `json:"old_role,omitempty"`
	NewRole   string `json:"new_role,omitempty"`
	PeerState string `json:"peer_state,omitempty"`
}

// RogueData carries rogue DHCP server detection information.
type RogueData struct {
	ServerIP  net.IP           `json:"server_ip"`
	ServerMAC net.HardwareAddr `json:"server_mac,omitempty"`
	OfferedIP net.IP           `json:"offered_ip,omitempty"`
	ClientMAC net.HardwareAddr `json:"client_mac,omitempty"`
	Interface string           `json:"interface,omitempty"`
	Count     int              `json:"count"`
}

// MarshalJSON implements custom JSON marshalling for Event.
func (e *Event) MarshalJSON() ([]byte, error) {
	type Alias Event
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	})
}

// ToEnvVars converts an event to environment variables for script hooks.
func (e *Event) ToEnvVars() map[string]string {
	env := map[string]string{
		"ATHENA_EVENT": string(e.Type),
	}

	if e.Lease != nil {
		l := e.Lease
		if l.IP != nil {
			env["ATHENA_IP"] = l.IP.String()
		}
		if l.MAC != nil {
			env["ATHENA_MAC"] = l.MAC.String()
		}
		if l.Hostname != "" {
			env["ATHENA_HOSTNAME"] = l.Hostname
		}
		if l.ClientID != "" {
			env["ATHENA_CLIENT_ID"] = l.ClientID
		}
		env["ATHENA_SUBNET"] = l.Subnet
		if l.Start != 0 {
			env["ATHENA_LEASE_START"] = fmt.Sprintf("%d", l.Start)
		}
		if l.Expiry != 0 {
			env["ATHENA_LEASE_EXPIRY"] = fmt.Sprintf("%d", l.Expiry)
		}
		if l.Start != 0 && l.Expiry != 0 {
			env["ATHENA_LEASE_DURATION"] = fmt.Sprintf("%d", l.Expiry-l.Start)
		}
		if l.FQDN != "" {
			env["ATHENA_FQDN"] = l.FQDN
		}
		if l.OldIP != nil {
			env["ATHENA_OLD_IP"] = l.OldIP.String()
		}
		if l.Pool != "" {
			env["ATHENA_POOL"] = l.Pool
		}
		if l.Relay != nil {
			if l.Relay.GIAddr != nil {
				env["ATHENA_GATEWAY"] = l.Relay.GIAddr.String()
			}
			if l.Relay.CircuitID != "" {
				env["ATHENA_RELAY_AGENT_CIRCUIT_ID"] = l.Relay.CircuitID
			}
			if l.Relay.RemoteID != "" {
				env["ATHENA_RELAY_AGENT_REMOTE_ID"] = l.Relay.RemoteID
			}
		}
		if opts, ok := l.Options["dns_servers"]; ok {
			env["ATHENA_DNS_SERVERS"] = fmt.Sprintf("%v", opts)
		}
		if opts, ok := l.Options["domain_name"]; ok {
			env["ATHENA_DOMAIN"] = fmt.Sprintf("%v", opts)
		}
	}

	if e.Conflict != nil {
		c := e.Conflict
		if c.IP != nil {
			env["ATHENA_IP"] = c.IP.String()
		}
		env["ATHENA_CONFLICT_METHOD"] = c.DetectionMethod
		if c.ResponderMAC != "" {
			env["ATHENA_CONFLICT_RESPONDER_MAC"] = c.ResponderMAC
		}
	}

	if e.Server != nil {
		env["ATHENA_SERVER_ID"] = e.Server.NodeID
	}

	return env
}
