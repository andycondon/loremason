package registry

import (
	"slices"
	"strings"
	"testing"
)

func TestSealedKeysOfTheShippedTypes(t *testing.T) {
	if s := typ(t, "decision").Sealed; !slices.Contains(s.Status, "accepted") || s.Via != "" {
		t.Errorf("decision: %+v", s)
	}
	if !typ(t, "question").Sealed.Settled {
		t.Error("a question is sealed once settled")
	}
	if s := typ(t, "step").Sealed; s.Via != "part_of" || !slices.Contains(s.Status, "accepted") {
		t.Errorf("step: %+v", s)
	}
}

func TestCheckSealedRefusesABadKey(t *testing.T) {
	r := Load()
	for _, c := range []struct {
		name string
		typ  string
		seal Sealed
		want string
	}{
		{"an unknown status", "decision", Sealed{Status: []string{"accepted", "done"}}, "sealed at `done`"},
		{"an unknown status through via", "step", Sealed{Via: "part_of", Status: []string{"held"}}, "sealed at `held`"},
		{"an unknown via edge", "step", Sealed{Via: "belongs_to", Status: []string{"accepted"}}, "sealed via `belongs_to`"},
	} {
		d := *typ(t, c.typ)
		d.Sealed = c.seal
		err := r.checkSealed(&d)
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), c.typ) {
			t.Errorf("%s: got %v, want one naming %s and %q", c.name, err, c.typ, c.want)
		}
	}
	for _, name := range r.Names() {
		if err := r.checkSealed(r.Types[name]); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
