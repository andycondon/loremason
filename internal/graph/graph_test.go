package graph

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/registry"
)

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
	for _, in := range []string{"", "a b", "@r1", "bel_7k3m9q@r0", "bel_7k3m9q@r", "bel_7k3m9q@x", "bel_7k3m9q@r1/s1", "bel_7k3m9q@r01", "bel_7k3m9q@r9999999999"} {
		if r, err := ParseRef(in); err == nil {
			t.Errorf("ParseRef(%q) = %+v, want an error", in, r)
		}
	}
}

func load(t *testing.T) *Graph {
	t.Helper()
	g, ds, err := Load("../../testdata/worked-example", registry.Load())
	if err != nil || len(ds) > 0 {
		t.Fatalf("%v %v", err, ds)
	}
	return g
}

func TestLoadWorkedExample(t *testing.T) {
	g := load(t)
	if len(g.Files) != 26 {
		t.Errorf("%d files", len(g.Files))
	}
	bel := g.Node("bel_7k3m9q")
	if bel == nil || bel.Type != "belief" || strings.Join(bel.Locals(), ",") != "s1,s2" {
		t.Fatalf("belief: %+v", bel)
	}
	s2 := bel.Child("s2")
	if s2.ID != "bel_7k3m9q/s2" || s2.Type != "support_link" || s2.Parent != bel || s2.Record() != bel {
		t.Errorf("sub-record: %+v", s2)
	}
	// Every edge resolves, and the ones out of the belief's sub-records start there.
	var fromSub int
	for _, e := range g.Edges {
		n := g.Resolve(e.To)
		if n == nil || !e.Def.Allows(n.Type) {
			t.Errorf("%s %s -> %s doesn't resolve to an allowed type", e.From.ID, e.Name, e.To)
		}
		if e.From.Parent == bel {
			fromSub++
		}
	}
	if fromSub != 2 {
		t.Errorf("%d edges from the belief's support links", fromSub)
	}
}

func TestResolve(t *testing.T) {
	g, _, err := Load("../cli/testdata/validate/alias-resolves", registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"bel_7k3m9q":      "bel_7k3m9q",
		"old/B-1":         "bel_7k3m9q", // an alias with a slash in it
		"B-2":             "bel_7k3m9q",
		"old/B-1/s1":      "bel_7k3m9q/s1", // a sub-record through an alias
		"bel_7k3m9q/s1":   "bel_7k3m9q/s1",
		"E-1":             "ev_p4n2xa",
		"bel_7k3m9q/s9":   "",
		"bel_7k3m9q/s1/x": "",
		"ev_p4n2xa/s1":    "",
		"nothing":         "",
	} {
		n := g.Resolve(Ref{Name: in})
		got := ""
		if n != nil {
			got = n.ID
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
	if p := g.Parent(Ref{Name: "bel_7k3m9q/s9"}); p == nil || p.ID != "bel_7k3m9q" {
		t.Errorf("Parent = %v", p)
	}
}

func TestPins(t *testing.T) {
	g, _, err := Load("../cli/testdata/validate/pinned-links", registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	var pins, vias int
	for _, e := range g.Edges {
		if e.Pin != nil {
			pins++
			if e.Pin.Rev < 1 || !strings.HasPrefix(e.Pin.FP, "fp1:") || e.Pin.Via == nil {
				t.Errorf("pin: %+v", e.Pin)
			}
		}
		if e.Name == "via" {
			vias++
		}
	}
	if pins != 2 || vias != 2 {
		t.Errorf("%d pins, %d via edges", pins, vias)
	}
}
