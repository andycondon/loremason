package id

import "testing"

func TestNewIsValid(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		s := New("bel")
		if p, ok := Prefix(s); !ok || p != "bel" {
			t.Fatalf("New made an invalid ID: %s", s)
		}
		seen[s] = true
	}
	if len(seen) < 999 {
		t.Fatalf("only %d distinct IDs in 1000", len(seen))
	}
}

func TestPrefix(t *testing.T) {
	for _, s := range []string{"", "bel", "bel_7k3m9", "bel_7k3m9qq", "bel_7k3m9i", "bel_7k3m9l", "bel_7k3m9o", "bel_7k3m9u", "BEL_7k3m9q", "bel_7k3m9q/s1"} {
		if _, ok := Prefix(s); ok {
			t.Errorf("%q was accepted", s)
		}
	}
}

func TestTrimRevision(t *testing.T) {
	for in, want := range map[string]string{
		"plan_k2x8wd@r2":  "plan_k2x8wd",
		"plan_k2x8wd@r10": "plan_k2x8wd",
		"plan_k2x8wd@r0":  "plan_k2x8wd@r0",
		"plan_k2x8wd":     "plan_k2x8wd",
	} {
		if got := TrimRevision(in); got != want {
			t.Errorf("TrimRevision(%q) = %q, want %q", in, got, want)
		}
	}
}
