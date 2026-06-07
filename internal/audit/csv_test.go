package audit

import "testing"

func TestCSVSafeNeutralisesFormulae(t *testing.T) {
	cases := map[string]string{
		"=cmd|' /c calc'!A1": "'=cmd|' /c calc'!A1",
		"+1+1":               "'+1+1",
		"-2+3":               "'-2+3",
		"@SUM(A1)":           "'@SUM(A1)",
		"\tTabbed":           "'\tTabbed",
		"normal-host":        "normal-host",
		"":                   "",
	}
	for in, want := range cases {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}
