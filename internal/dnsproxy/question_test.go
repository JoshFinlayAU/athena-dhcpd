package dnsproxy

import (
	"testing"

	"github.com/miekg/dns"
)

func mkMsg(name string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	return m
}

func TestSameQuestion(t *testing.T) {
	q := mkMsg("example.com", dns.TypeA)

	if !sameQuestion(q, mkMsg("example.com", dns.TypeA)) {
		t.Error("identical question should match")
	}
	if !sameQuestion(q, mkMsg("EXAMPLE.COM", dns.TypeA)) {
		t.Error("case should be ignored")
	}
	if sameQuestion(q, mkMsg("evil.com", dns.TypeA)) {
		t.Error("different name must not match")
	}
	if sameQuestion(q, mkMsg("example.com", dns.TypeAAAA)) {
		t.Error("different type must not match")
	}
	if sameQuestion(q, new(dns.Msg)) {
		t.Error("empty response question must not match")
	}
}
