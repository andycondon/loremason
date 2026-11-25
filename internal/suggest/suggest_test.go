package suggest

import "testing"

func TestNear(t *testing.T) {
	fields := []string{"relies_on", "part_of", "status", "title"}
	for in, want := range map[string]string{
		"relies_om": "relies_on",
		"stauts":    "status",
		"partof":    "part_of",
		"banana":    "",
	} {
		if got := Near(in, fields); got != want {
			t.Errorf("Near(%q) = %q, want %q", in, got, want)
		}
	}
}
