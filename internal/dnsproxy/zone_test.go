package dnsproxy

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestNewZone(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		wantFQDN string
	}{
		{"bare domain", "example.com", "example.com."},
		{"already fqdn", "example.com.", "example.com."},
		{"empty", "", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			z := NewZone(tt.domain, 60)
			if z.Domain() != tt.wantFQDN {
				t.Errorf("Domain() = %q, want %q", z.Domain(), tt.wantFQDN)
			}
			if z.Count() != 0 {
				t.Errorf("Count() = %d, want 0", z.Count())
			}
		})
	}
}

func TestZoneAddAndLookup(t *testing.T) {
	z := NewZone("example.com", 60)

	a := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	z.Add(a)

	if z.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", z.Count())
	}

	// Lookup should find it
	rrs := z.Lookup("host.example.com.", dns.TypeA)
	if len(rrs) != 1 {
		t.Fatalf("Lookup returned %d records, want 1", len(rrs))
	}

	aRec, ok := rrs[0].(*dns.A)
	if !ok {
		t.Fatal("expected *dns.A record")
	}
	if !aRec.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("A record IP = %s, want 10.0.0.1", aRec.A)
	}

	// Case insensitive lookup
	rrs = z.Lookup("HOST.EXAMPLE.COM.", dns.TypeA)
	if len(rrs) != 1 {
		t.Errorf("case-insensitive lookup returned %d records, want 1", len(rrs))
	}

	// Wrong type returns nil
	rrs = z.Lookup("host.example.com.", dns.TypeAAAA)
	if len(rrs) != 0 {
		t.Errorf("wrong-type lookup returned %d records, want 0", len(rrs))
	}
}

func TestZoneAddReplaces(t *testing.T) {
	z := NewZone("example.com", 60)

	a1 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	a2 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.2").To4(),
	}

	z.Add(a1)
	z.Add(a2) // Should replace, not append

	if z.Count() != 1 {
		t.Fatalf("Count() = %d after replacement, want 1", z.Count())
	}

	rrs := z.Lookup("host.example.com.", dns.TypeA)
	aRec := rrs[0].(*dns.A)
	if !aRec.A.Equal(net.ParseIP("10.0.0.2")) {
		t.Errorf("after replacement, IP = %s, want 10.0.0.2", aRec.A)
	}
}

func TestZoneAddMulti(t *testing.T) {
	z := NewZone("example.com", 60)

	a1 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	a2 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.2").To4(),
	}

	z.AddMulti(a1)
	z.AddMulti(a2)

	if z.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", z.Count())
	}

	rrs := z.Lookup("host.example.com.", dns.TypeA)
	if len(rrs) != 2 {
		t.Errorf("Lookup returned %d records, want 2", len(rrs))
	}
}

func TestZoneRemove(t *testing.T) {
	z := NewZone("example.com", 60)

	a := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	z.Add(a)

	z.Remove("host.example.com.", dns.TypeA)
	if z.Count() != 0 {
		t.Errorf("Count() after remove = %d, want 0", z.Count())
	}
	if z.Has("host.example.com.", dns.TypeA) {
		t.Error("Has() returned true after removal")
	}
}

func TestZoneRemoveByValue(t *testing.T) {
	z := NewZone("example.com", 60)

	a1 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	a2 := &dns.A{
		Hdr: dns.RR_Header{Name: "host.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.2").To4(),
	}
	z.AddMulti(a1)
	z.AddMulti(a2)

	z.RemoveByValue("host.example.com.", dns.TypeA, "10.0.0.1")

	if z.Count() != 1 {
		t.Fatalf("Count() = %d after RemoveByValue, want 1", z.Count())
	}
	rrs := z.Lookup("host.example.com.", dns.TypeA)
	aRec := rrs[0].(*dns.A)
	if !aRec.A.Equal(net.ParseIP("10.0.0.2")) {
		t.Errorf("remaining IP = %s, want 10.0.0.2", aRec.A)
	}
}

func TestZoneRegisterLease(t *testing.T) {
	z := NewZone("example.com", 300)
	ip := net.ParseIP("192.168.1.50")

	z.RegisterLease("myhost", ip, true)

	// Check A record
	rrs := z.Lookup("myhost.example.com.", dns.TypeA)
	if len(rrs) != 1 {
		t.Fatalf("A record lookup returned %d records, want 1", len(rrs))
	}
	aRec := rrs[0].(*dns.A)
	if !aRec.A.Equal(ip) {
		t.Errorf("A record IP = %s, want %s", aRec.A, ip)
	}

	// Check PTR record
	rrs = z.Lookup("50.1.168.192.in-addr.arpa.", dns.TypePTR)
	if len(rrs) != 1 {
		t.Fatalf("PTR record lookup returned %d records, want 1", len(rrs))
	}
	ptrRec := rrs[0].(*dns.PTR)
	if ptrRec.Ptr != "myhost.example.com." {
		t.Errorf("PTR target = %q, want %q", ptrRec.Ptr, "myhost.example.com.")
	}
}

func TestZoneRegisterLeaseNoPTR(t *testing.T) {
	z := NewZone("example.com", 300)
	ip := net.ParseIP("192.168.1.50")

	z.RegisterLease("myhost", ip, false)

	// A record should exist
	if !z.Has("myhost.example.com.", dns.TypeA) {
		t.Error("A record should exist")
	}

	// PTR should NOT exist
	if z.Has("50.1.168.192.in-addr.arpa.", dns.TypePTR) {
		t.Error("PTR record should not exist when addPTR=false")
	}
}

func TestZoneUnregisterLease(t *testing.T) {
	z := NewZone("example.com", 300)
	ip := net.ParseIP("192.168.1.50")

	z.RegisterLease("myhost", ip, true)
	z.UnregisterLease("myhost", ip)

	if z.Has("myhost.example.com.", dns.TypeA) {
		t.Error("A record should be removed after UnregisterLease")
	}
	if z.Has("50.1.168.192.in-addr.arpa.", dns.TypePTR) {
		t.Error("PTR record should be removed after UnregisterLease")
	}
}

func TestZoneAllRecords(t *testing.T) {
	z := NewZone("example.com", 60)

	a := &dns.A{
		Hdr: dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("10.0.0.1").To4(),
	}
	ptr := &dns.PTR{
		Hdr: dns.RR_Header{Name: "1.0.0.10.in-addr.arpa.", Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 60},
		Ptr: "a.example.com.",
	}
	z.Add(a)
	z.Add(ptr)

	all := z.AllRecords()
	if len(all) != 2 {
		t.Errorf("AllRecords() returned %d, want 2", len(all))
	}
}

func TestParseStaticRecord(t *testing.T) {
	tests := []struct {
		name    string
		rname   string
		rtype   string
		value   string
		ttl     uint32
		wantErr bool
	}{
		{"valid A", "host.example.com", "A", "10.0.0.1", 60, false},
		{"valid AAAA", "host.example.com", "AAAA", "2001:db8::1", 60, false},
		{"valid CNAME", "alias.example.com", "CNAME", "host.example.com", 60, false},
		{"valid PTR", "1.0.0.10.in-addr.arpa", "PTR", "host.example.com", 60, false},
		{"valid TXT", "host.example.com", "TXT", "v=spf1 include:example.com", 60, false},
		{"valid MX", "example.com", "MX", "10 mail.example.com", 60, false},
		{"valid SRV", "_sip._tcp.example.com", "SRV", "10 60 5060 sip.example.com", 60, false},
		{"invalid A", "host.example.com", "A", "not-an-ip", 60, true},
		{"invalid AAAA with v4", "host.example.com", "AAAA", "10.0.0.1", 60, true},
		{"invalid MX format", "example.com", "MX", "bad", 60, true},
		{"invalid SRV format", "_sip._tcp.example.com", "SRV", "bad", 60, true},
		{"unsupported type", "host.example.com", "NS", "ns1.example.com", 60, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr, err := ParseStaticRecord(tt.rname, tt.rtype, tt.value, tt.ttl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (rr=%v)", rr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if rr == nil {
				t.Error("expected non-nil RR")
			}
		})
	}
}

func TestReverseIP(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"192.168.1.1", "1.1.168.192"},
		{"10.0.0.1", "1.0.0.10"},
		{"255.255.255.0", "0.255.255.255"},
	}

	for _, tt := range tests {
		got := reverseIP(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("reverseIP(%s) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}
