// Package dhcpv4 provides constants and encoding helpers for DHCPv4 packets.
package dhcpv4

import "net"

// DHCP Message Types (RFC 2131 §9.6)
type MessageType byte

const (
	MessageTypeDiscover MessageType = 1 // DHCPDISCOVER
	MessageTypeOffer    MessageType = 2 // DHCPOFFER
	MessageTypeRequest  MessageType = 3 // DHCPREQUEST
	MessageTypeDecline  MessageType = 4 // DHCPDECLINE
	MessageTypeAck      MessageType = 5 // DHCPACK
	MessageTypeNak      MessageType = 6 // DHCPNAK
	MessageTypeRelease  MessageType = 7 // DHCPRELEASE
	MessageTypeInform   MessageType = 8 // DHCPINFORM
)

func (m MessageType) String() string {
	switch m {
	case MessageTypeDiscover:
		return "DHCPDISCOVER"
	case MessageTypeOffer:
		return "DHCPOFFER"
	case MessageTypeRequest:
		return "DHCPREQUEST"
	case MessageTypeDecline:
		return "DHCPDECLINE"
	case MessageTypeAck:
		return "DHCPACK"
	case MessageTypeNak:
		return "DHCPNAK"
	case MessageTypeRelease:
		return "DHCPRELEASE"
	case MessageTypeInform:
		return "DHCPINFORM"
	default:
		return "UNKNOWN"
	}
}

// DHCP Op Codes (RFC 2131 §2)
type OpCode byte

const (
	OpCodeBootRequest OpCode = 1 // BOOTREQUEST
	OpCodeBootReply   OpCode = 2 // BOOTREPLY
)

// Hardware Types (RFC 1700)
type HardwareType byte

const (
	HardwareTypeEthernet HardwareType = 1
)

// DHCP Option Codes (RFC 2132 and extensions)
type OptionCode byte

const (
	OptionPad                    OptionCode = 0
	OptionSubnetMask             OptionCode = 1
	OptionTimeOffset             OptionCode = 2
	OptionRouter                 OptionCode = 3
	OptionTimeServer             OptionCode = 4
	OptionNameServer             OptionCode = 5
	OptionDomainNameServer       OptionCode = 6
	OptionLogServer              OptionCode = 7
	OptionCookieServer           OptionCode = 8
	OptionLPRServer              OptionCode = 9
	OptionImpressServer          OptionCode = 10
	OptionResourceLocationServer OptionCode = 11
	OptionHostname               OptionCode = 12
	OptionBootFileSize           OptionCode = 13
	OptionMeritDumpFile          OptionCode = 14
	OptionDomainName             OptionCode = 15
	OptionSwapServer             OptionCode = 16
	OptionRootPath               OptionCode = 17
	OptionExtensionsPath         OptionCode = 18
	OptionIPForwarding           OptionCode = 19
	OptionNonLocalSourceRouting  OptionCode = 20
	OptionPolicyFilter           OptionCode = 21
	OptionMaxDatagramReassembly  OptionCode = 22
	OptionDefaultIPTTL           OptionCode = 23
	OptionPathMTUAgingTimeout    OptionCode = 24
	OptionPathMTUPlateauTable    OptionCode = 25
	OptionInterfaceMTU           OptionCode = 26
	OptionAllSubnetsLocal        OptionCode = 27
	OptionBroadcastAddress       OptionCode = 28
	OptionPerformMaskDiscovery   OptionCode = 29
	OptionMaskSupplier           OptionCode = 30
	OptionPerformRouterDiscovery OptionCode = 31
	OptionRouterSolicitAddr      OptionCode = 32
	OptionStaticRoute            OptionCode = 33
	OptionTrailerEncapsulation   OptionCode = 34
	OptionARPCacheTimeout        OptionCode = 35
	OptionEthernetEncapsulation  OptionCode = 36
	OptionTCPDefaultTTL          OptionCode = 37
	OptionTCPKeepaliveInterval   OptionCode = 38
	OptionTCPKeepaliveGarbage    OptionCode = 39
	OptionNISDomain              OptionCode = 40
	OptionNISServers             OptionCode = 41
	OptionNTPServers             OptionCode = 42
	OptionVendorSpecific         OptionCode = 43
	OptionNetBIOSNameServer      OptionCode = 44
	OptionNetBIOSDatagramDist    OptionCode = 45
	OptionNetBIOSNodeType        OptionCode = 46
	OptionNetBIOSScope           OptionCode = 47
	OptionXWindowFontServer      OptionCode = 48
	OptionXWindowDisplayManager  OptionCode = 49
	OptionRequestedIP            OptionCode = 50
	OptionIPLeaseTime            OptionCode = 51
	OptionOverload               OptionCode = 52
	OptionDHCPMessageType        OptionCode = 53
	OptionServerIdentifier       OptionCode = 54
	OptionParameterRequestList   OptionCode = 55
	OptionMessage                OptionCode = 56
	OptionMaxDHCPMessageSize     OptionCode = 57
	OptionRenewalTime            OptionCode = 58
	OptionRebindingTime          OptionCode = 59
	OptionVendorClassID          OptionCode = 60
	OptionClientIdentifier       OptionCode = 61
	OptionNetWareIPDomain        OptionCode = 62
	OptionNetWareIPOption        OptionCode = 63
	OptionTFTPServerName         OptionCode = 66
	OptionBootfileName           OptionCode = 67
	OptionUserClass              OptionCode = 77
	OptionClientFQDN             OptionCode = 81
	OptionRelayAgentInfo         OptionCode = 82
	OptionSubnetSelection        OptionCode = 118
	OptionClasslessStaticRoute   OptionCode = 121
	OptionVIVendorClass          OptionCode = 124
	OptionVIVendorSpecific       OptionCode = 125
	OptionTFTPServerAddress      OptionCode = 150
	OptionEnd                    OptionCode = 255
)

// Relay Agent Information Sub-Option Types (RFC 3046)
const (
	RelaySubOptionCircuitID  byte = 1
	RelaySubOptionRemoteID   byte = 2
	RelaySubOptionLinkSelect byte = 5 // RFC 3527
)

// DHCP Packet Size Limits
const (
	MinPacketSize     = 300  // Minimum DHCP packet size (RFC 2131)
	MaxPacketSize     = 1500 // Maximum DHCP packet size (Ethernet MTU)
	DefaultPacketSize = 576  // Default max packet size (RFC 2131 §2)
)

// DHCP Ports
const (
	ServerPort = 67
	ClientPort = 68
)

// DHCP Magic Cookie (RFC 2131 §3)
var MagicCookie = []byte{99, 130, 83, 99}

// Broadcast MAC and IP
var (
	BroadcastMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	BroadcastIP  = net.IPv4(255, 255, 255, 255)
	ZeroIP       = net.IPv4(0, 0, 0, 0)
)

// Lease States
type LeaseState string

const (
	LeaseStateActive   LeaseState = "active"
	LeaseStateOffered  LeaseState = "offered"
	LeaseStateExpired  LeaseState = "expired"
	LeaseStateReleased LeaseState = "released"
	LeaseStateDeclined LeaseState = "declined"
)

// Conflict Detection Methods
type DetectionMethod string

const (
	DetectionARPProbe      DetectionMethod = "arp_probe"
	DetectionICMPProbe     DetectionMethod = "icmp_probe"
	DetectionClientDecline DetectionMethod = "client_decline"
)

// HA Failover States
type HAState string

const (
	HAStatePartnerUp   HAState = "PARTNER_UP"
	HAStatePartnerDown HAState = "PARTNER_DOWN"
	HAStateActive      HAState = "ACTIVE"
	HAStateStandby     HAState = "STANDBY"
	HAStateRecovery    HAState = "RECOVERY"
)

// HA Message Types
type HAMessageType byte

const (
	HAMsgHeartbeat     HAMessageType = 0x01
	HAMsgLeaseUpdate   HAMessageType = 0x02
	HAMsgBulkStart     HAMessageType = 0x03
	HAMsgBulkData      HAMessageType = 0x04
	HAMsgBulkEnd       HAMessageType = 0x05
	HAMsgFailoverClaim HAMessageType = 0x06
	HAMsgFailoverAck   HAMessageType = 0x07
	HAMsgStateRequest  HAMessageType = 0x08
	HAMsgConflictUpdate HAMessageType = 0x09
	HAMsgConflictBulk  HAMessageType = 0x0A
)
