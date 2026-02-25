package dhcp

import (
	"fmt"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// RelayAgentInfo holds parsed Option 82 sub-options (RFC 3046).
type RelayAgentInfo struct {
	CircuitID  string
	RemoteID   string
	LinkSelect []byte // RFC 3527 sub-option 5
	Raw        []byte
}

// ParseRelayAgentInfo decodes Option 82 sub-options from raw bytes.
// RFC 3046 — Relay Agent Information Option.
func ParseRelayAgentInfo(data []byte) (*RelayAgentInfo, error) {
	info := &RelayAgentInfo{Raw: data}
	i := 0
	for i < len(data) {
		if i+1 >= len(data) {
			return nil, fmt.Errorf("truncated relay agent sub-option at offset %d", i)
		}
		subType := data[i]
		subLen := int(data[i+1])
		i += 2
		if i+subLen > len(data) {
			return nil, fmt.Errorf("truncated relay agent sub-option %d at offset %d", subType, i-2)
		}
		subData := data[i : i+subLen]
		i += subLen

		switch subType {
		case dhcpv4.RelaySubOptionCircuitID:
			info.CircuitID = string(subData)
		case dhcpv4.RelaySubOptionRemoteID:
			info.RemoteID = string(subData)
		case dhcpv4.RelaySubOptionLinkSelect:
			info.LinkSelect = make([]byte, len(subData))
			copy(info.LinkSelect, subData)
		}
	}
	return info, nil
}

// EncodeRelayAgentInfo encodes relay agent sub-options to bytes.
func EncodeRelayAgentInfo(info *RelayAgentInfo) []byte {
	var buf []byte
	if info.CircuitID != "" {
		buf = append(buf, dhcpv4.RelaySubOptionCircuitID)
		buf = append(buf, byte(len(info.CircuitID)))
		buf = append(buf, []byte(info.CircuitID)...)
	}
	if info.RemoteID != "" {
		buf = append(buf, dhcpv4.RelaySubOptionRemoteID)
		buf = append(buf, byte(len(info.RemoteID)))
		buf = append(buf, []byte(info.RemoteID)...)
	}
	if len(info.LinkSelect) > 0 {
		buf = append(buf, dhcpv4.RelaySubOptionLinkSelect)
		buf = append(buf, byte(len(info.LinkSelect)))
		buf = append(buf, info.LinkSelect...)
	}
	return buf
}

// GetRelayInfo extracts relay agent info from a packet's Option 82.
func GetRelayInfo(pkt *Packet) *RelayAgentInfo {
	data, ok := pkt.Options[dhcpv4.OptionRelayAgentInfo]
	if !ok {
		return nil
	}
	info, err := ParseRelayAgentInfo(data)
	if err != nil {
		return nil
	}
	return info
}
