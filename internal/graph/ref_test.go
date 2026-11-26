package graph

import (
	"testing"

	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/schema"
)

var badRefs = []string{"", "a b", "a\tb", "@r1", "bel_7k3m9q@r0", "bel_7k3m9q@r", "bel_7k3m9q@x", "bel_7k3m9q@r1/s1", "bel_7k3m9q@r01", "bel_7k3m9q@r9999999999", "a@b@r1"}

func TestParseRef(t *testing.T) {
	for in, want := range map[string]Ref{
		"bel_7k3m9q":        {Name: "bel_7k3m9q"},
		"plan_k2x8wd@r2":    {Name: "plan_k2x8wd", Rev: 2},
		"bel_7k3m9q/s1@r12": {Name: "bel_7k3m9q/s1", Rev: 12},
		"old/B-1":           {Name: "old/B-1"},
	} {
		got, err := ParseRef(in)
		if err != nil || got != want || got.String() != in {
			t.Errorf("ParseRef(%q) = %+v, %v", in, got, err)
		}
	}
	for _, in := range badRefs {
		if r, err := ParseRef(in); err == nil {
			t.Errorf("ParseRef(%q) = %+v, want an error", in, r)
		}
	}
}

// ParseRef and the schemas' ref pattern must agree, or a link could be
// skipped by the loader without the schema check reporting it.
func TestParseRefAgreesWithTheSchema(t *testing.T) {
	refSchema := registry.Load().SubschemaAt(schema.BaseURL + "common.json#/$defs/ref")
	for _, s := range append(badRefs, "bel_7k3m9q", "old/B-1@r3", "x y", "bel_7k3m9q/s1@r12") {
		_, err := ParseRef(s)
		if schemaOK := refSchema.Validate(s) == nil; schemaOK != (err == nil) {
			t.Errorf("%q: the schema says %v, ParseRef says %v", s, schemaOK, err)
		}
	}
}

func TestSubrecordID(t *testing.T) {
	if got := SubrecordID("bel_7k3m9q", "s1"); got != "bel_7k3m9q/s1" {
		t.Errorf("SubrecordID = %q", got)
	}
}

func TestSplitSubrecord(t *testing.T) {
	for _, c := range []struct {
		in, record, local string
		ok                bool
	}{
		{"bel_7k3m9q/s1", "bel_7k3m9q", "s1", true},
		{"old/B-1/s1", "old/B-1", "s1", true}, // at the last slash
		{"bel_7k3m9q", "bel_7k3m9q", "", false},
	} {
		record, local, ok := splitSubrecord(c.in)
		if record != c.record || local != c.local || ok != c.ok {
			t.Errorf("splitSubrecord(%q) = %q, %q, %v", c.in, record, local, ok)
		}
	}
}
