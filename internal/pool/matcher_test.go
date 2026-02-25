package pool

import (
	"net"
	"testing"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		// Exact match
		{"eth0/1", "eth0/1", true},
		{"eth0/1", "eth0/2", false},

		// Wildcard *
		{"eth0/*", "eth0/1", true},
		{"eth0/*", "eth0/anything", true},
		{"Cisco*", "CiscoSwitch", true},
		{"Cisco*", "JuniperRouter", false},

		// Single character ?
		{"eth?/1", "eth0/1", true},
		{"eth?/1", "eth1/1", true},
		{"eth?/1", "ethab/1", false},

		// Character class []
		{"eth[012]/1", "eth0/1", true},
		{"eth[012]/1", "eth3/1", false},

		// Empty value never matches
		{"*", "", false},
		{"eth0", "", false},

		// Exact match with no wildcards
		{"myswitch", "myswitch", true},
		{"myswitch", "otherswitch", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.value, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func newMatcherPool(t *testing.T, name string, circuitID, remoteID, vendorClass, userClass string) *Pool {
	t.Helper()
	_, network, _ := net.ParseCIDR("10.0.0.0/24")
	p, err := NewPool(name, net.IPv4(10, 0, 0, 10), net.IPv4(10, 0, 0, 50), network)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	p.MatchCircuitID = circuitID
	p.MatchRemoteID = remoteID
	p.MatchVendorClass = vendorClass
	p.MatchUserClass = userClass
	return p
}

func TestPoolMatches(t *testing.T) {
	tests := []struct {
		name     string
		pool     func(t *testing.T) *Pool
		criteria MatchCriteria
		want     bool
	}{
		{
			name: "no criteria matches everything",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "default", "", "", "", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/1", RemoteID: "sw1", VendorClass: "Cisco"},
			want:     true,
		},
		{
			name: "circuit_id exact match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "vlan10", "eth0/1", "", "", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/1"},
			want:     true,
		},
		{
			name: "circuit_id mismatch",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "vlan10", "eth0/1", "", "", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/2"},
			want:     false,
		},
		{
			name: "circuit_id glob match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "vlan10", "eth0/*", "", "", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/5"},
			want:     true,
		},
		{
			name: "remote_id match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "sw1pool", "", "switch-1", "", "")
			},
			criteria: MatchCriteria{RemoteID: "switch-1"},
			want:     true,
		},
		{
			name: "remote_id mismatch",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "sw1pool", "", "switch-1", "", "")
			},
			criteria: MatchCriteria{RemoteID: "switch-2"},
			want:     false,
		},
		{
			name: "vendor_class match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "cisco", "", "", "Cisco*", "")
			},
			criteria: MatchCriteria{VendorClass: "CiscoAP"},
			want:     true,
		},
		{
			name: "vendor_class mismatch",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "cisco", "", "", "Cisco*", "")
			},
			criteria: MatchCriteria{VendorClass: "Juniper"},
			want:     false,
		},
		{
			name: "user_class match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "voip", "", "", "", "VOIP*")
			},
			criteria: MatchCriteria{UserClass: "VOIP-Phone"},
			want:     true,
		},
		{
			name: "user_class mismatch",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "voip", "", "", "", "VOIP*")
			},
			criteria: MatchCriteria{UserClass: "Workstation"},
			want:     false,
		},
		{
			name: "multiple criteria all match",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "specific", "eth0/*", "sw1", "Cisco*", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/3", RemoteID: "sw1", VendorClass: "CiscoAP"},
			want:     true,
		},
		{
			name: "multiple criteria partial mismatch",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "specific", "eth0/*", "sw1", "Cisco*", "")
			},
			criteria: MatchCriteria{CircuitID: "eth0/3", RemoteID: "sw2", VendorClass: "CiscoAP"},
			want:     false,
		},
		{
			name: "criteria requires value but client has none",
			pool: func(t *testing.T) *Pool {
				return newMatcherPool(t, "circuit", "eth0/1", "", "", "")
			},
			criteria: MatchCriteria{},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.pool(t)
			got := p.Matches(tt.criteria)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasMatchCriteria(t *testing.T) {
	tests := []struct {
		name string
		pool *Pool
		want bool
	}{
		{"no criteria", &Pool{}, false},
		{"circuit_id only", &Pool{MatchCircuitID: "eth0"}, true},
		{"remote_id only", &Pool{MatchRemoteID: "sw1"}, true},
		{"vendor_class only", &Pool{MatchVendorClass: "Cisco"}, true},
		{"user_class only", &Pool{MatchUserClass: "VOIP"}, true},
		{"all criteria", &Pool{MatchCircuitID: "x", MatchRemoteID: "y", MatchVendorClass: "z", MatchUserClass: "w"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pool.HasMatchCriteria(); got != tt.want {
				t.Errorf("HasMatchCriteria() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectPool(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.0.0.0/24")

	makePool := func(name, circuitID, vendorClass string) *Pool {
		p, _ := NewPool(name, net.IPv4(10, 0, 0, 10), net.IPv4(10, 0, 0, 50), network)
		p.MatchCircuitID = circuitID
		p.MatchVendorClass = vendorClass
		return p
	}

	defaultPool := makePool("default", "", "")
	ciscoPool := makePool("cisco", "", "Cisco*")
	vlan10Pool := makePool("vlan10", "eth0/1*", "")

	pools := []*Pool{ciscoPool, vlan10Pool, defaultPool}

	t.Run("matches specific pool by vendor", func(t *testing.T) {
		got := SelectPool(pools, MatchCriteria{VendorClass: "CiscoAP"})
		if got != ciscoPool {
			t.Errorf("expected ciscoPool, got %v", got)
		}
	})

	t.Run("matches specific pool by circuit_id", func(t *testing.T) {
		got := SelectPool(pools, MatchCriteria{CircuitID: "eth0/10"})
		if got != vlan10Pool {
			t.Errorf("expected vlan10Pool, got %v", got)
		}
	})

	t.Run("falls back to default pool", func(t *testing.T) {
		got := SelectPool(pools, MatchCriteria{VendorClass: "Juniper"})
		if got != defaultPool {
			t.Errorf("expected defaultPool, got %v", got)
		}
	})

	t.Run("falls back to default with empty criteria", func(t *testing.T) {
		got := SelectPool(pools, MatchCriteria{})
		if got != defaultPool {
			t.Errorf("expected defaultPool, got %v", got)
		}
	})

	t.Run("nil if no pools", func(t *testing.T) {
		got := SelectPool(nil, MatchCriteria{})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("nil if no default and no match", func(t *testing.T) {
		onlyCisco := []*Pool{ciscoPool}
		got := SelectPool(onlyCisco, MatchCriteria{VendorClass: "Juniper"})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("first matching pool wins", func(t *testing.T) {
		cisco1 := makePool("cisco1", "", "Cisco*")
		cisco2 := makePool("cisco2", "", "Cisco*")
		got := SelectPool([]*Pool{cisco1, cisco2}, MatchCriteria{VendorClass: "CiscoSwitch"})
		if got != cisco1 {
			t.Errorf("expected first matching pool (cisco1), got %v", got.Name)
		}
	})
}
