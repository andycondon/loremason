package registry

import (
	"slices"
	"strings"
	"testing"
)

// Decisions, questions and steps are sealed as types.yaml says.
func TestSealedKeysOfTheShippedTypes(t *testing.T) {
	if s := typ(t, "decision").Sealed; !slices.Contains(s.Status, "accepted") || typ(t, "decision").StatusFrom != "" {
		t.Errorf("decision: %+v", s)
	}
	if !typ(t, "question").Sealed.Settled {
		t.Error("a question is sealed once settled")
	}
	if st := typ(t, "step"); st.StatusFrom != "part_of" || !slices.Contains(st.Sealed.Status, "accepted") {
		t.Errorf("step: %+v", st.Sealed)
	}
}

// A sealed key with an unknown status or via edge is refused.
func TestCheckSealedRefusesABadKey(t *testing.T) {
	r := Load()
	for _, c := range []struct {
		name string
		typ  string
		seal Sealed
		from string
		want string
	}{
		{"an unknown status", "decision", Sealed{Status: []string{"accepted", "done"}}, "", "sealed at `done`"},
		{"an unknown status through status_from", "step", Sealed{Status: []string{"held"}}, "part_of", "sealed at `held`"},
		{"an unknown status_from edge", "step", Sealed{Status: []string{"accepted"}}, "belongs_to", "status from `belongs_to`"},
		{"a status_from edge that holds a list", "step", Sealed{Status: []string{"accepted"}}, "follows", "is a list"},
	} {
		d := *typ(t, c.typ)
		d.Sealed, d.StatusFrom = c.seal, c.from
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

func TestEvidenceIsFixed(t *testing.T) {
	if !typ(t, "evidence").Fixed {
		t.Error("evidence never changes, so it is fixed")
	}
}

func TestCheckFixedRefusesWhatCannotKeepAFingerprint(t *testing.T) {
	d := *typ(t, "finding")
	if !d.Revisable {
		t.Fatal("the test needs a revisable type")
	}
	d.Fixed = true
	if err := d.checkFixed(); err == nil || !strings.Contains(err.Error(), "finding") || !strings.Contains(err.Error(), "revisable") {
		t.Errorf("fixed on a revisable type: %v", err)
	}

	d = *typ(t, "initiative")
	if d.Revisable || d.Schema.Properties["fp"] != nil {
		t.Fatal("the test needs a plain type without fp")
	}
	d.Fixed = true
	if err := d.checkFixed(); err == nil || !strings.Contains(err.Error(), "initiative") || !strings.Contains(err.Error(), "fp") {
		t.Errorf("fixed on a schema without fp: %v", err)
	}

	e := *typ(t, "evidence")
	if err := e.checkFixed(); err != nil {
		t.Errorf("evidence: %v", err)
	}
}
