package portauto

import (
	"log/slog"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestEvaluateSimpleMAC(t *testing.T) {
	e := NewEngine(testLogger())
	err := e.SetRules([]Rule{
		{
			Name:        "block-known-bad",
			Enabled:     true,
			MACPatterns: []string{`^aa:bb:cc`},
			Actions:     []Action{{Type: "log"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	results := e.Evaluate(LeaseContext{MAC: "aa:bb:cc:dd:ee:ff"})
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Rule != "block-known-bad" {
		t.Errorf("rule = %q", results[0].Rule)
	}

	// Non-matching MAC
	results = e.Evaluate(LeaseContext{MAC: "11:22:33:44:55:66"})
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestEvaluateSubnetFilter(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{
			Name:    "vlan-assign",
			Enabled: true,
			Subnets: []string{"10.0.0.0/24"},
			Actions: []Action{{Type: "tag", VLAN: 100}},
		},
	})

	results := e.Evaluate(LeaseContext{Subnet: "10.0.0.0/24", MAC: "aa:bb:cc:00:00:01"})
	if len(results) != 1 {
		t.Fatalf("expected 1 match")
	}

	results = e.Evaluate(LeaseContext{Subnet: "192.168.1.0/24", MAC: "aa:bb:cc:00:00:01"})
	if len(results) != 0 {
		t.Errorf("wrong subnet should not match")
	}
}

func TestEvaluateDeviceType(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{
			Name:        "quarantine-cameras",
			Enabled:     true,
			DeviceTypes: []string{"camera"},
			Actions:     []Action{{Type: "webhook", URL: "http://example.com/quarantine"}},
		},
	})

	results := e.Evaluate(LeaseContext{DeviceType: "Camera", MAC: "aa:00:00:00:00:01"})
	if len(results) != 1 {
		t.Fatalf("device type match should be case-insensitive")
	}

	results = e.Evaluate(LeaseContext{DeviceType: "computer", MAC: "aa:00:00:00:00:01"})
	if len(results) != 0 {
		t.Errorf("non-matching device type should not match")
	}
}

func TestEvaluateCircuitID(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{
			Name:       "server-room",
			Enabled:    true,
			CircuitIDs: []string{`^eth0/1/`},
			Actions:    []Action{{Type: "log"}},
		},
	})

	results := e.Evaluate(LeaseContext{CircuitID: "eth0/1/3", MAC: "aa:00:00:00:00:01"})
	if len(results) != 1 {
		t.Fatalf("circuit-id regex should match")
	}

	results = e.Evaluate(LeaseContext{CircuitID: "eth0/2/1", MAC: "aa:00:00:00:00:01"})
	if len(results) != 0 {
		t.Errorf("non-matching circuit-id should not match")
	}
}

func TestEvaluateANDLogic(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{
			Name:        "strict-rule",
			Enabled:     true,
			Subnets:     []string{"10.0.0.0/24"},
			DeviceTypes: []string{"printer"},
			Actions:     []Action{{Type: "tag", VLAN: 200}},
		},
	})

	// Both match
	results := e.Evaluate(LeaseContext{Subnet: "10.0.0.0/24", DeviceType: "printer", MAC: "aa:00:00:00:00:01"})
	if len(results) != 1 {
		t.Error("both criteria match, should get result")
	}

	// Only subnet matches
	results = e.Evaluate(LeaseContext{Subnet: "10.0.0.0/24", DeviceType: "computer", MAC: "aa:00:00:00:00:01"})
	if len(results) != 0 {
		t.Error("AND logic: only subnet match should not trigger")
	}
}

func TestDisabledRule(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{
			Name:    "disabled",
			Enabled: false,
			Subnets: []string{"10.0.0.0/24"},
			Actions: []Action{{Type: "log"}},
		},
	})

	results := e.Evaluate(LeaseContext{Subnet: "10.0.0.0/24"})
	if len(results) != 0 {
		t.Error("disabled rule should not match")
	}
}

func TestMultipleRules(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{Name: "rule1", Enabled: true, Subnets: []string{"10.0.0.0/24"}, Actions: []Action{{Type: "log"}}},
		{Name: "rule2", Enabled: true, DeviceTypes: []string{"printer"}, Actions: []Action{{Type: "log"}}},
	})

	// Both should match
	results := e.Evaluate(LeaseContext{Subnet: "10.0.0.0/24", DeviceType: "printer"})
	if len(results) != 2 {
		t.Errorf("expected 2 matches, got %d", len(results))
	}
}

func TestInvalidRegex(t *testing.T) {
	e := NewEngine(testLogger())
	err := e.SetRules([]Rule{
		{Name: "bad", Enabled: true, MACPatterns: []string{"[invalid"}},
	})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestGetRulesReturnsCopy(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{Name: "test", Enabled: true, Actions: []Action{{Type: "log"}}},
	})

	rules := e.GetRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	if rules[0].macRE != nil {
		t.Error("compiled regexes should not be exposed")
	}
}

func TestExecuteLog(t *testing.T) {
	e := NewEngine(testLogger())
	e.SetRules([]Rule{
		{Name: "log-all", Enabled: true, Subnets: []string{"10.0.0.0/24"}, Actions: []Action{{Type: "log"}}},
	})

	// Should not panic
	results := e.Execute(LeaseContext{Subnet: "10.0.0.0/24", MAC: "aa:00:00:00:00:01", IP: "10.0.0.50"})
	if len(results) != 1 {
		t.Errorf("expected 1 result")
	}
}
