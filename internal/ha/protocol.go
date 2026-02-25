// Package ha provides high availability via peer lease synchronisation.
package ha

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// Message is the wire-format message exchanged between HA peers.
type Message struct {
	Type      dhcpv4.HAMessageType `json:"type"`
	Timestamp int64                `json:"timestamp"`
	Payload   json.RawMessage      `json:"payload,omitempty"`
}

// HeartbeatPayload is sent periodically to confirm peer liveness.
type HeartbeatPayload struct {
	State       string `json:"state"`
	LeaseCount  int    `json:"lease_count"`
	Seq         uint64 `json:"seq"`
	Uptime      int64  `json:"uptime_seconds"`
}

// LeaseUpdatePayload carries a single lease change to the peer.
type LeaseUpdatePayload struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	ClientID  string `json:"client_id,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	Subnet    string `json:"subnet"`
	Pool      string `json:"pool,omitempty"`
	State     string `json:"state"`
	Start     int64  `json:"start"`
	Expiry    int64  `json:"expiry"`
	Seq       uint64 `json:"seq"`
}

// BulkStartPayload signals the beginning of a bulk sync.
type BulkStartPayload struct {
	TotalLeases    int `json:"total_leases"`
	TotalConflicts int `json:"total_conflicts"`
}

// BulkEndPayload signals the completion of a bulk sync.
type BulkEndPayload struct {
	LeasesTransferred    int `json:"leases_transferred"`
	ConflictsTransferred int `json:"conflicts_transferred"`
}

// ConflictUpdatePayload carries a conflict table entry to the peer.
type ConflictUpdatePayload struct {
	IP              string `json:"ip"`
	DetectedAt      int64  `json:"detected_at"`
	DetectionMethod string `json:"detection_method"`
	ResponderMAC    string `json:"responder_mac,omitempty"`
	Subnet          string `json:"subnet"`
	ProbeCount      int    `json:"probe_count"`
	Permanent       bool   `json:"permanent"`
}

// FailoverClaimPayload is sent when a node claims active role.
type FailoverClaimPayload struct {
	Reason    string `json:"reason"`
	ClaimedAt int64  `json:"claimed_at"`
}

// EncodeMessage serializes a Message to bytes with a length prefix (4-byte big-endian).
func EncodeMessage(msg *Message) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshalling HA message: %w", err)
	}

	// Length-prefixed frame: [4 bytes length][JSON payload]
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)

	return frame, nil
}

// DecodeMessage reads a length-prefixed Message from a reader.
func DecodeMessage(r io.Reader) (*Message, error) {
	// Read 4-byte length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("reading message length: %w", err)
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > 1<<20 { // 1 MB max message size
		return nil, fmt.Errorf("message too large: %d bytes", msgLen)
	}

	// Read message body
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}

	msg := &Message{}
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("unmarshalling HA message: %w", err)
	}

	return msg, nil
}

// NewHeartbeat creates a heartbeat message.
func NewHeartbeat(state string, leaseCount int, seq uint64, uptime time.Duration) (*Message, error) {
	payload, err := json.Marshal(HeartbeatPayload{
		State:      state,
		LeaseCount: leaseCount,
		Seq:        seq,
		Uptime:     int64(uptime.Seconds()),
	})
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      dhcpv4.HAMsgHeartbeat,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}, nil
}

// NewLeaseUpdate creates a lease update message.
func NewLeaseUpdate(ip net.IP, mac net.HardwareAddr, clientID, hostname, subnet, pool, state string,
	start, expiry time.Time, seq uint64) (*Message, error) {
	payload, err := json.Marshal(LeaseUpdatePayload{
		IP:       ip.String(),
		MAC:      mac.String(),
		ClientID: clientID,
		Hostname: hostname,
		Subnet:   subnet,
		Pool:     pool,
		State:    state,
		Start:    start.Unix(),
		Expiry:   expiry.Unix(),
		Seq:      seq,
	})
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      dhcpv4.HAMsgLeaseUpdate,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}, nil
}

// NewConflictUpdate creates a conflict update message.
func NewConflictUpdate(ip net.IP, detectedAt time.Time, method, responderMAC, subnet string,
	probeCount int, permanent bool) (*Message, error) {
	payload, err := json.Marshal(ConflictUpdatePayload{
		IP:              ip.String(),
		DetectedAt:      detectedAt.Unix(),
		DetectionMethod: method,
		ResponderMAC:    responderMAC,
		Subnet:          subnet,
		ProbeCount:      probeCount,
		Permanent:       permanent,
	})
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      dhcpv4.HAMsgConflictUpdate,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}, nil
}
