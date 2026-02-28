package macvendor

import (
	"log/slog"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLookupFromJSON(t *testing.T) {
	data, err := os.ReadFile("../../macdb/macdb.json")
	if err != nil {
		t.Skip("macdb.json not found, skipping")
	}

	db := NewDB(testLogger())
	if err := db.Load(data); err != nil {
		t.Fatal(err)
	}

	if db.Count() == 0 {
		t.Fatal("no entries loaded")
	}
	t.Logf("loaded %d vendor entries", db.Count())

	// Cisco is 00:00:0C
	vendor := db.Lookup("00:00:0C:11:22:33")
	if vendor != "Cisco Systems, Inc" {
		t.Errorf("Cisco lookup = %q", vendor)
	}

	// Test with different formats
	vendor2 := db.Lookup("00-00-0C-11-22-33")
	if vendor2 != "Cisco Systems, Inc" {
		t.Errorf("Cisco dash format = %q", vendor2)
	}

	vendor3 := db.Lookup("00000c112233")
	if vendor3 != "Cisco Systems, Inc" {
		t.Errorf("Cisco no-sep format = %q", vendor3)
	}
}

func TestLookupUnknown(t *testing.T) {
	db := NewDB(testLogger())
	db.Load([]byte(`[{"macPrefix":"AA:BB:CC","vendorName":"TestCorp","private":false,"blockType":"MA-L"}]`))

	if v := db.Lookup("AA:BB:CC:DD:EE:FF"); v != "TestCorp" {
		t.Errorf("expected TestCorp, got %q", v)
	}

	if v := db.Lookup("FF:FF:FF:DD:EE:FF"); v != "" {
		t.Errorf("unknown should return empty, got %q", v)
	}
}

func TestLookupShortMAC(t *testing.T) {
	db := NewDB(testLogger())
	db.Load([]byte(`[{"macPrefix":"AA:BB:CC","vendorName":"TestCorp","private":false,"blockType":"MA-L"}]`))

	// Too short
	if v := db.Lookup("AA"); v != "" {
		t.Errorf("short MAC should return empty, got %q", v)
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"00:00:0C", "00000c"},
		{"00-00-0C", "00000c"},
		{"AA:BB:CC:DD", "aabbccdd"},
		{"aabbcc", "aabbcc"},
	}
	for _, tc := range tests {
		got := normalizePrefix(tc.input)
		if got != tc.want {
			t.Errorf("normalizePrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestLoadBadJSON(t *testing.T) {
	db := NewDB(testLogger())
	if err := db.Load([]byte("not json")); err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestEmptyDB(t *testing.T) {
	db := NewDB(testLogger())
	if db.Count() != 0 {
		t.Errorf("empty DB count = %d", db.Count())
	}
	if v := db.Lookup("AA:BB:CC:DD:EE:FF"); v != "" {
		t.Errorf("empty DB lookup = %q", v)
	}
}
