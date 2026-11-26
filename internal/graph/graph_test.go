package graph

import (
	"slices"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/registry"
)

// names is a small graph with every kind of name: IDs, aliases (one with a
// slash in it), and sub-records.
func names(t *testing.T) *Graph {
	return load(t,
		file("bel_7k3m9q", "aliases: [old/B-1, B-2]", "support:", "  - {id: s1}", "  - {id: s2}"),
		file("ev_p4n2xa", "aliases: [E-1]"),
	)
}

// ---- Graph ----

func TestGraphNode(t *testing.T) {
	g := names(t)
	if g.Node("bel_7k3m9q") == nil || g.Node("bel_7k3m9q/s1") == nil {
		t.Error("IDs and sub-record IDs are nodes")
	}
	if g.Node("B-2") != nil {
		t.Error("Node doesn't resolve aliases")
	}
}

func TestGraphResolve(t *testing.T) {
	g := names(t)
	for in, want := range map[string]string{
		"bel_7k3m9q":      "bel_7k3m9q",
		"old/B-1":         "bel_7k3m9q", // an alias with a slash in it
		"B-2":             "bel_7k3m9q",
		"old/B-1/s1":      "bel_7k3m9q/s1", // a sub-record through an alias
		"bel_7k3m9q/s1":   "bel_7k3m9q/s1",
		"E-1":             "ev_p4n2xa",
		"bel_7k3m9q/s9":   "", // no such sub-record
		"bel_7k3m9q/s1/x": "", // sub-records have no sub-records
		"ev_p4n2xa/s1":    "",
		"nothing":         "",
	} {
		got := ""
		if n := g.Resolve(Ref{Name: in}); n != nil {
			got = n.ID
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGraphParent(t *testing.T) {
	g := names(t)
	for in, want := range map[string]string{
		"bel_7k3m9q/s9":   "bel_7k3m9q", // whether or not the sub-record exists
		"old/B-1/s1":      "bel_7k3m9q",
		"bel_7k3m9q/s1/x": "", // the part before the slash is a sub-record
		"bel_7k3m9q":      "", // no slash
		"nothing/s1":      "",
	} {
		got := ""
		if p := g.Parent(Ref{Name: in}); p != nil {
			got = p.ID
		}
		if got != want {
			t.Errorf("Parent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGraphLookup(t *testing.T) {
	g := names(t)
	if n, err := g.Lookup("old/B-1@r1"); err != nil || n.ID != "bel_7k3m9q" {
		t.Errorf("an alias with a revision: %v, %v", n, err)
	}
	for in, want := range map[string]string{
		"bel_nope00": "no record is called `bel_nope00`",
		"bel@r0":     "bel@r0", // not a reference
	} {
		if _, err := g.Lookup(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Lookup(%q): %v", in, err)
		}
	}
}

func TestGraphNames(t *testing.T) {
	want := []string{"B-2", "E-1", "bel_7k3m9q", "bel_7k3m9q/s1", "bel_7k3m9q/s2", "ev_p4n2xa", "old/B-1"}
	if got := names(t).Names(); !slices.Equal(got, want) {
		t.Errorf("Names = %v", got)
	}
}

// ---- Node ----

func TestNodeShape(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "support:", "  - {id: s1}", "  - {id: s2}"),
		file("who_4d9snc"),
	)
	bel, s2, actor := g.Node("bel_7k3m9q"), g.Node("bel_7k3m9q/s2"), g.Node("who_4d9snc")
	if bel.IsSubrecord() || !s2.IsSubrecord() {
		t.Error("IsSubrecord")
	}
	if !bel.Revisable() || actor.Revisable() || s2.Revisable() {
		t.Error("a belief keeps revisions; an actor and a sub-record don't")
	}
	if bel.Record() != bel || s2.Record() != bel {
		t.Error("Record is the node itself, or its parent")
	}
	if bel.Child("s2") != s2 || bel.Child("s9") != nil || !slices.Equal(bel.Locals(), []string{"s1", "s2"}) {
		t.Errorf("Child and Locals: %v", bel.Locals())
	}
}

func TestNodeRevisions(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "revisions:", "  - {rev: 1}", "  - {rev: 2}"),
		file("who_4d9snc"),
	)
	bel := g.Node("bel_7k3m9q")
	if bel.Current() != &bel.Revisions[1] || bel.Revision(1) != &bel.Revisions[0] || bel.Revision(3) != nil {
		t.Errorf("revisions: %+v", bel.Revisions)
	}
	if actor := g.Node("who_4d9snc"); actor.Current() != nil || actor.Revision(1) != nil {
		t.Error("a node that keeps no revisions has none")
	}
}

func TestNodeSealedAndPinnedBy(t *testing.T) {
	pin := func(rev string) string { return "  - {to: bel_7k3m9q, rev: " + rev + ", fp: \"fp1:000000000000\"}" }
	bel := load(t,
		file("bel_7k3m9q", "revisions:", "  - {rev: 1}", "  - {rev: 2}"),
		file("task_9w4hzd", "relies_on:", pin("1"), pin("1")),
		file("task_2b4cdf", "relies_on:", pin("1"), "  - bel_7k3m9q"),
	).Node("bel_7k3m9q")
	if !bel.Sealed(1) || bel.Sealed(2) {
		t.Error("only the pinned revision is sealed; a bare link seals nothing")
	}
	// Each record once, however many of its links pin the revision.
	if got := bel.PinnedBy(1); !slices.Equal(got, []string{"task_2b4cdf", "task_9w4hzd"}) {
		t.Errorf("PinnedBy(1) = %v", got)
	}
	if got := bel.PinnedBy(2); got != nil {
		t.Errorf("PinnedBy(2) = %v", got)
	}
}

func TestNodeIdentity(t *testing.T) {
	g := load(t,
		file("art_7j3mdr", "url: https://example.com/a"),
		file("art_9xk2pq", "url: \"  HTTPS://example.com/A  \""),
		file("art_3nb6vw"),
		file("ev_p4n2xa"),
	)
	a, ok := g.Node("art_7j3mdr").Identity()
	if !ok || !slices.Equal(a.Fields, []string{"url"}) || a.At != (diag.Pos{Path: "graph/artifact/art_7j3mdr.md", Line: 4}) {
		t.Fatalf("identity: %+v, %v", a, ok)
	}
	// Case and spacing don't count.
	if b, _ := g.Node("art_9xk2pq").Identity(); b.Key != a.Key {
		t.Errorf("keys differ: %q and %q", a.Key, b.Key)
	}
	if _, ok := g.Node("art_3nb6vw").Identity(); ok {
		t.Error("an empty identity field means no identity")
	}
	if _, ok := g.Node("ev_p4n2xa").Identity(); ok {
		t.Error("evidence has no identity fields")
	}
}

// ---- Edge ----

func TestEdgePinnable(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "support:", "  - {id: s1, evidence: ev_p4n2xa}"),
		file("ev_p4n2xa"),
		file("ini_2m8vqe"),
		file("task_9w4hzd", "relies_on: bel_7k3m9q", "part_of: ini_2m8vqe", "derived_from: dd_nope00"),
	)
	if !edge(t, g, "task_9w4hzd", "relies_on").Pinnable() {
		t.Error("relies_on propagates, and a belief keeps revisions")
	}
	if edge(t, g, "task_9w4hzd", "part_of").Pinnable() {
		t.Error("part_of doesn't propagate")
	}
	if edge(t, g, "task_9w4hzd", "derived_from").Pinnable() {
		t.Error("a link to nothing can't be pinned")
	}
	// No sub-record edge in the registry propagates, so make one that would.
	fromSub := &Edge{From: g.Node("bel_7k3m9q/s1"), Target: g.Node("bel_7k3m9q"), Def: registry.Edge{To: []string{"any"}, Propagates: true}}
	if fromSub.Pinnable() {
		t.Error("a sub-record's links are never pinned")
	}
}

// stale is a belief at revision 2, and a task whose link pins it.
func stale(t *testing.T, rev, fp string) *Edge {
	g := load(t,
		file("bel_7k3m9q", "claim: a claim", "revisions:", "  - {rev: 1}", "  - {rev: 2}"),
		file("task_9w4hzd", "relies_on:", "  - {to: bel_7k3m9q, rev: "+rev+", fp: \""+fp+"\"}"),
	)
	return edge(t, g, "task_9w4hzd", "relies_on")
}

func TestEdgeStaleAndBehind(t *testing.T) {
	current := load(t, file("bel_7k3m9q", "claim: a claim")).Node("bel_7k3m9q").Fingerprint().String()
	for _, c := range []struct {
		rev, fp string
		stale   bool
		behind  int
	}{
		{"2", current, false, 0},           // the current revision and meaning
		{"1", current, true, 1},            // an older revision
		{"2", "fp1:222222222222", true, 0}, // the meaning changed since
	} {
		e := stale(t, c.rev, c.fp)
		if e.Stale() != c.stale || len(e.Behind()) != c.behind {
			t.Errorf("rev %s, %s: stale %v, behind %d", c.rev, c.fp, e.Stale(), len(e.Behind()))
		}
	}
	bare := load(t, file("bel_7k3m9q"), file("task_9w4hzd", "relies_on: [bel_7k3m9q, bel_nope00]"))
	for _, e := range bare.Edges {
		if e.Stale() || e.Behind() != nil {
			t.Errorf("an unpinned link, or one to nothing, is never stale: %s", e.To)
		}
	}
}

// ---- File ----

func TestFileAt(t *testing.T) {
	f := load(t, file("bel_7k3m9q")).Files[0]
	if f.At(3) != (diag.Pos{Path: "graph/belief/bel_7k3m9q.md", Line: 3}) {
		t.Errorf("At(3) = %+v", f.At(3))
	}
}

func TestFileMisplaced(t *testing.T) {
	g, _ := loadFiles(t, map[string]string{"belief/bel_7k3m9q.md": file("bel_7k3m9q"), "question/bel_2wq8dn.md": file("bel_2wq8dn")})
	if g.Node("bel_7k3m9q").File.Misplaced() || !g.Node("bel_2wq8dn").File.Misplaced() {
		t.Error("a file is misplaced when its ID's prefix and its folder disagree")
	}
}
