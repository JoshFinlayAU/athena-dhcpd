package dhcp

import (
	"fmt"
	"net"

	"github.com/athena-dhcpd/athena-dhcpd/pkg/dhcpv4"
)

// OptionType defines the data type of a DHCP option.
type OptionType int

const (
	TypeIP         OptionType = iota // Single IPv4 address (4 bytes)
	TypeIPList                       // Multiple IPv4 addresses (N*4 bytes)
	TypeUint8                        // Single byte
	TypeUint16                       // 2 bytes big-endian
	TypeUint32                       // 4 bytes big-endian
	TypeInt32                        // 4 bytes big-endian signed
	TypeBool                         // 1 byte, 0x00 or 0x01
	TypeString                       // Variable-length ASCII
	TypeBytes                        // Raw bytes
	TypeIPMask                       // IP + subnet mask pairs
	TypeCIDRRoutes                   // RFC 3442 encoded routes
	TypeIPPairs                      // IP address pairs (N*8 bytes)
	TypeUint16List                   // Multiple uint16 values
)

// OptionDef defines a DHCP option's metadata for the registry.
type OptionDef struct {
	Code     dhcpv4.OptionCode
	Name     string
	Type     OptionType
	MinLen   int
	MaxLen   int
	Multiple bool // Can appear multiple times
}

// optionRegistry maps option codes to their definitions.
var optionRegistry = map[dhcpv4.OptionCode]OptionDef{
	dhcpv4.OptionSubnetMask:             {Code: 1, Name: "Subnet Mask", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionTimeOffset:             {Code: 2, Name: "Time Offset", Type: TypeInt32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionRouter:                 {Code: 3, Name: "Router", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionTimeServer:             {Code: 4, Name: "Time Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionNameServer:             {Code: 5, Name: "Name Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionDomainNameServer:       {Code: 6, Name: "Domain Name Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionLogServer:              {Code: 7, Name: "Log Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionCookieServer:           {Code: 8, Name: "Cookie Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionLPRServer:              {Code: 9, Name: "LPR Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionImpressServer:          {Code: 10, Name: "Impress Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionResourceLocationServer: {Code: 11, Name: "Resource Location Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionHostname:               {Code: 12, Name: "Host Name", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionBootFileSize:           {Code: 13, Name: "Boot File Size", Type: TypeUint16, MinLen: 2, MaxLen: 2},
	dhcpv4.OptionMeritDumpFile:          {Code: 14, Name: "Merit Dump File", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionDomainName:             {Code: 15, Name: "Domain Name", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionSwapServer:             {Code: 16, Name: "Swap Server", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionRootPath:               {Code: 17, Name: "Root Path", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionExtensionsPath:         {Code: 18, Name: "Extensions Path", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionIPForwarding:           {Code: 19, Name: "IP Forwarding", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionNonLocalSourceRouting:  {Code: 20, Name: "Non-Local Source Routing", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionPolicyFilter:           {Code: 21, Name: "Policy Filter", Type: TypeIPPairs, MinLen: 8, MaxLen: 252},
	dhcpv4.OptionMaxDatagramReassembly:  {Code: 22, Name: "Max Datagram Reassembly Size", Type: TypeUint16, MinLen: 2, MaxLen: 2},
	dhcpv4.OptionDefaultIPTTL:           {Code: 23, Name: "Default IP TTL", Type: TypeUint8, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionPathMTUAgingTimeout:    {Code: 24, Name: "Path MTU Aging Timeout", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionPathMTUPlateauTable:    {Code: 25, Name: "Path MTU Plateau Table", Type: TypeUint16List, MinLen: 2, MaxLen: 252},
	dhcpv4.OptionInterfaceMTU:           {Code: 26, Name: "Interface MTU", Type: TypeUint16, MinLen: 2, MaxLen: 2},
	dhcpv4.OptionAllSubnetsLocal:        {Code: 27, Name: "All Subnets Local", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionBroadcastAddress:       {Code: 28, Name: "Broadcast Address", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionPerformMaskDiscovery:   {Code: 29, Name: "Perform Mask Discovery", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionMaskSupplier:           {Code: 30, Name: "Mask Supplier", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionPerformRouterDiscovery: {Code: 31, Name: "Perform Router Discovery", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionRouterSolicitAddr:      {Code: 32, Name: "Router Solicitation Address", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionStaticRoute:            {Code: 33, Name: "Static Route", Type: TypeIPPairs, MinLen: 8, MaxLen: 252},
	dhcpv4.OptionTrailerEncapsulation:   {Code: 34, Name: "Trailer Encapsulation", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionARPCacheTimeout:        {Code: 35, Name: "ARP Cache Timeout", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionEthernetEncapsulation:  {Code: 36, Name: "Ethernet Encapsulation", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionTCPDefaultTTL:          {Code: 37, Name: "TCP Default TTL", Type: TypeUint8, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionTCPKeepaliveInterval:   {Code: 38, Name: "TCP Keepalive Interval", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionTCPKeepaliveGarbage:    {Code: 39, Name: "TCP Keepalive Garbage", Type: TypeBool, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionNISDomain:              {Code: 40, Name: "NIS Domain", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionNISServers:             {Code: 41, Name: "NIS Servers", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionNTPServers:             {Code: 42, Name: "NTP Servers", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionVendorSpecific:         {Code: 43, Name: "Vendor Specific", Type: TypeBytes, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionNetBIOSNameServer:      {Code: 44, Name: "NetBIOS Name Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionNetBIOSDatagramDist:    {Code: 45, Name: "NetBIOS Datagram Distribution", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionNetBIOSNodeType:        {Code: 46, Name: "NetBIOS Node Type", Type: TypeUint8, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionNetBIOSScope:           {Code: 47, Name: "NetBIOS Scope", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionXWindowFontServer:      {Code: 48, Name: "X Window Font Server", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionXWindowDisplayManager:  {Code: 49, Name: "X Window Display Manager", Type: TypeIPList, MinLen: 4, MaxLen: 252},
	dhcpv4.OptionRequestedIP:            {Code: 50, Name: "Requested IP", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionIPLeaseTime:            {Code: 51, Name: "IP Lease Time", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionOverload:               {Code: 52, Name: "Overload", Type: TypeUint8, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionDHCPMessageType:        {Code: 53, Name: "DHCP Message Type", Type: TypeUint8, MinLen: 1, MaxLen: 1},
	dhcpv4.OptionServerIdentifier:       {Code: 54, Name: "Server Identifier", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionParameterRequestList:   {Code: 55, Name: "Parameter Request List", Type: TypeBytes, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionMessage:                {Code: 56, Name: "Message", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionMaxDHCPMessageSize:     {Code: 57, Name: "Max DHCP Message Size", Type: TypeUint16, MinLen: 2, MaxLen: 2},
	dhcpv4.OptionRenewalTime:            {Code: 58, Name: "Renewal Time (T1)", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionRebindingTime:          {Code: 59, Name: "Rebinding Time (T2)", Type: TypeUint32, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionVendorClassID:          {Code: 60, Name: "Vendor Class Identifier", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionClientIdentifier:       {Code: 61, Name: "Client Identifier", Type: TypeBytes, MinLen: 2, MaxLen: 255},
	dhcpv4.OptionTFTPServerName:         {Code: 66, Name: "TFTP Server Name", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionBootfileName:           {Code: 67, Name: "Bootfile Name", Type: TypeString, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionUserClass:              {Code: 77, Name: "User Class", Type: TypeBytes, MinLen: 1, MaxLen: 255},
	dhcpv4.OptionClientFQDN:             {Code: 81, Name: "Client FQDN", Type: TypeBytes, MinLen: 3, MaxLen: 255},
	dhcpv4.OptionRelayAgentInfo:         {Code: 82, Name: "Relay Agent Information", Type: TypeBytes, MinLen: 2, MaxLen: 255},
	dhcpv4.OptionSubnetSelection:        {Code: 118, Name: "Subnet Selection", Type: TypeIP, MinLen: 4, MaxLen: 4},
	dhcpv4.OptionClasslessStaticRoute:   {Code: 121, Name: "Classless Static Route", Type: TypeCIDRRoutes, MinLen: 5, MaxLen: 255},
	dhcpv4.OptionTFTPServerAddress:      {Code: 150, Name: "TFTP Server Address", Type: TypeIPList, MinLen: 4, MaxLen: 252},
}

// GetOptionDef returns the definition for an option code, or nil if unknown.
func GetOptionDef(code dhcpv4.OptionCode) *OptionDef {
	def, ok := optionRegistry[code]
	if !ok {
		return nil
	}
	return &def
}

// ValidateOption checks that raw option data matches the expected type constraints.
func ValidateOption(code dhcpv4.OptionCode, data []byte) error {
	def := GetOptionDef(code)
	if def == nil {
		// Unknown option — accept as raw bytes
		return nil
	}
	if len(data) < def.MinLen {
		return fmt.Errorf("option %d (%s): data too short (%d < %d)", code, def.Name, len(data), def.MinLen)
	}
	if def.MaxLen > 0 && len(data) > def.MaxLen {
		return fmt.Errorf("option %d (%s): data too long (%d > %d)", code, def.Name, len(data), def.MaxLen)
	}

	switch def.Type {
	case TypeIP:
		if len(data) != 4 {
			return fmt.Errorf("option %d (%s): expected 4 bytes for IP, got %d", code, def.Name, len(data))
		}
	case TypeIPList:
		if len(data)%4 != 0 {
			return fmt.Errorf("option %d (%s): IP list length %d not multiple of 4", code, def.Name, len(data))
		}
	case TypeUint16:
		if len(data) != 2 {
			return fmt.Errorf("option %d (%s): expected 2 bytes for uint16, got %d", code, def.Name, len(data))
		}
	case TypeUint32, TypeInt32:
		if len(data) != 4 {
			return fmt.Errorf("option %d (%s): expected 4 bytes for uint32/int32, got %d", code, def.Name, len(data))
		}
	case TypeBool:
		if len(data) != 1 {
			return fmt.Errorf("option %d (%s): expected 1 byte for bool, got %d", code, def.Name, len(data))
		}
	}

	return nil
}

// BuildOptionsFromConfig creates an Options map from subnet/pool/reservation config values.
func BuildOptionsFromConfig(subnetMask net.IPMask, routers, dnsServers, ntpServers []net.IP,
	domainName, hostname, tftpServer, bootfile string,
	leaseTime, renewalTime, rebindTime uint32,
	broadcast net.IP) Options {

	opts := make(Options)

	if subnetMask != nil {
		opts[dhcpv4.OptionSubnetMask] = []byte(subnetMask)
	}
	if len(routers) > 0 {
		opts[dhcpv4.OptionRouter] = dhcpv4.IPListToBytes(routers)
	}
	if len(dnsServers) > 0 {
		opts[dhcpv4.OptionDomainNameServer] = dhcpv4.IPListToBytes(dnsServers)
	}
	if len(ntpServers) > 0 {
		opts[dhcpv4.OptionNTPServers] = dhcpv4.IPListToBytes(ntpServers)
	}
	if domainName != "" {
		opts[dhcpv4.OptionDomainName] = []byte(domainName)
	}
	if hostname != "" {
		opts[dhcpv4.OptionHostname] = []byte(hostname)
	}
	if tftpServer != "" {
		opts[dhcpv4.OptionTFTPServerName] = []byte(tftpServer)
	}
	if bootfile != "" {
		opts[dhcpv4.OptionBootfileName] = []byte(bootfile)
	}
	if leaseTime > 0 {
		opts.SetUint32(dhcpv4.OptionIPLeaseTime, leaseTime)
	}
	if renewalTime > 0 {
		opts.SetUint32(dhcpv4.OptionRenewalTime, renewalTime)
	}
	if rebindTime > 0 {
		opts.SetUint32(dhcpv4.OptionRebindingTime, rebindTime)
	}
	if broadcast != nil {
		opts[dhcpv4.OptionBroadcastAddress] = dhcpv4.IPToBytes(broadcast)
	}

	return opts
}
