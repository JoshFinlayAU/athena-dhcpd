// Package ddns provides dynamic DNS update integration for athena-dhcpd.
package ddns

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/miekg/dns"
)

// RFC2136Client performs DNS updates using RFC 2136 (DNS UPDATE) with optional TSIG.
type RFC2136Client struct {
	server    string
	tsigName  string
	tsigAlgo  string
	tsigKey   string
	timeout   time.Duration
	logger    *slog.Logger
}

// NewRFC2136Client creates a new RFC 2136 DNS update client.
func NewRFC2136Client(server, tsigName, tsigAlgo, tsigKey string, timeout time.Duration, logger *slog.Logger) *RFC2136Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &RFC2136Client{
		server:   server,
		tsigName: tsigName,
		tsigAlgo: tsigAlgo,
		tsigKey:  tsigKey,
		timeout:  timeout,
		logger:   logger,
	}
}

// AddA adds or updates an A record via DNS UPDATE.
func (c *RFC2136Client) AddA(zone, fqdn string, ip net.IP, ttl uint32) error {
	msg := c.newUpdateMsg(zone)

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(fqdn),
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		A: ip.To4(),
	}

	// Remove existing A records, then add new one
	rrRemove := &dns.A{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(fqdn),
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
		},
	}
	msg.RemoveRRset([]dns.RR{rrRemove})
	msg.Insert([]dns.RR{rr})

	return c.send(msg, "AddA", fqdn, ip.String())
}

// RemoveA removes an A record via DNS UPDATE.
func (c *RFC2136Client) RemoveA(zone, fqdn string) error {
	msg := c.newUpdateMsg(zone)

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(fqdn),
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
		},
	}
	msg.RemoveRRset([]dns.RR{rr})

	return c.send(msg, "RemoveA", fqdn, "")
}

// AddPTR adds or updates a PTR record via DNS UPDATE.
func (c *RFC2136Client) AddPTR(zone, reverseIP, fqdn string, ttl uint32) error {
	msg := c.newUpdateMsg(zone)

	ptrName := reverseIP + "."
	rr := &dns.PTR{
		Hdr: dns.RR_Header{
			Name:   ptrName,
			Rrtype: dns.TypePTR,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		Ptr: dns.Fqdn(fqdn),
	}

	// Remove existing PTR records, then add
	rrRemove := &dns.PTR{
		Hdr: dns.RR_Header{
			Name:   ptrName,
			Rrtype: dns.TypePTR,
			Class:  dns.ClassANY,
		},
	}
	msg.RemoveRRset([]dns.RR{rrRemove})
	msg.Insert([]dns.RR{rr})

	return c.send(msg, "AddPTR", reverseIP, fqdn)
}

// RemovePTR removes a PTR record via DNS UPDATE.
func (c *RFC2136Client) RemovePTR(zone, reverseIP string) error {
	msg := c.newUpdateMsg(zone)

	ptrName := reverseIP + "."
	rr := &dns.PTR{
		Hdr: dns.RR_Header{
			Name:   ptrName,
			Rrtype: dns.TypePTR,
			Class:  dns.ClassANY,
		},
	}
	msg.RemoveRRset([]dns.RR{rr})

	return c.send(msg, "RemovePTR", reverseIP, "")
}

// AddDHCID adds a DHCID record for conflict detection (RFC 4701).
func (c *RFC2136Client) AddDHCID(zone, fqdn string, dhcid []byte, ttl uint32) error {
	msg := c.newUpdateMsg(zone)

	rr := &dns.DHCID{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(fqdn),
			Rrtype: dns.TypeDHCID,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		Digest: string(dhcid),
	}
	msg.Insert([]dns.RR{rr})

	return c.send(msg, "AddDHCID", fqdn, "")
}

// newUpdateMsg creates a new DNS UPDATE message for the given zone.
func (c *RFC2136Client) newUpdateMsg(zone string) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetUpdate(dns.Fqdn(zone))
	return msg
}

// send transmits a DNS UPDATE message with optional TSIG signing.
func (c *RFC2136Client) send(msg *dns.Msg, op, name, value string) error {
	client := &dns.Client{
		Timeout: c.timeout,
		Net:     "tcp",
	}

	// Apply TSIG signing if configured
	if c.tsigName != "" && c.tsigKey != "" {
		algo := c.tsigAlgorithm()
		msg.SetTsig(c.tsigName, algo, 300, time.Now().Unix())
		client.TsigSecret = map[string]string{c.tsigName: c.tsigKey}
	}

	start := time.Now()
	resp, _, err := client.Exchange(msg, c.server)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("DNS UPDATE failed",
			"op", op,
			"name", name,
			"server", c.server,
			"error", err,
			"duration", duration.String())
		return fmt.Errorf("DNS UPDATE %s for %s: %w", op, name, err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		c.logger.Error("DNS UPDATE rejected",
			"op", op,
			"name", name,
			"server", c.server,
			"rcode", dns.RcodeToString[resp.Rcode],
			"duration", duration.String())
		return fmt.Errorf("DNS UPDATE %s for %s: server returned %s", op, name, dns.RcodeToString[resp.Rcode])
	}

	c.logger.Debug("DNS UPDATE success",
		"op", op,
		"name", name,
		"value", value,
		"server", c.server,
		"duration", duration.String())

	return nil
}

// tsigAlgorithm returns the TSIG algorithm string for miekg/dns.
func (c *RFC2136Client) tsigAlgorithm() string {
	switch c.tsigAlgo {
	case "hmac-sha256", "":
		return dns.HmacSHA256
	case "hmac-sha512":
		return dns.HmacSHA512
	case "hmac-sha1":
		return dns.HmacSHA1
	case "hmac-md5":
		return dns.HmacMD5
	default:
		return dns.HmacSHA256
	}
}
