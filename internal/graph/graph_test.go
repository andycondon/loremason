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

func TestNodeSealedAndSealedBy(t *testing.T) {
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
	if got := bel.SealedBy(1); !slices.Equal(got, []string{"task_2b4cdf", "task_9w4hzd"}) {
		t.Errorf("SealedBy(1) = %v", got)
	}
	if got := bel.SealedBy(2); got != nil {
		t.Errorf("SealedBy(2) = %v", got)
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

// ---- Node: status ----

func TestNodeStatus(t *testing.T) {
	g := load(t, file("bel_7k3m9q", "status: incorrect"), file("ev_p4n2xa"))
	if g.Node("bel_7k3m9q").Status() != "incorrect" || g.Node("ev_p4n2xa").Status() != "" {
		t.Error("Status is the header's status, or empty")
	}
}

// ---- Edge: reopens ----

// reopens returns whether the edge from a record through relies_on, or
// another field, reopens its source.
func reopens(t *testing.T, g *Graph, from, name string) bool {
	t.Helper()
	return edge(t, g, from, name).Reopens()
}

func TestEdgeReopensWhenABeliefHasFallen(t *testing.T) {
	for _, status := range []string{"incorrect", "insufficient"} {
		g := load(t,
			file("bel_7k3m9q", "status: "+status),
			file("dec_9w4hzd", "relies_on: [bel_7k3m9q]"),
			file("task_p4n2xa", "relies_on: [bel_7k3m9q]"),
		)
		if !reopens(t, g, "dec_9w4hzd", "relies_on") || !reopens(t, g, "task_p4n2xa", "relies_on") {
			t.Errorf("a decision and a task relying on a belief that is %s reopen", status)
		}
	}
}

func TestEdgeFromAClosedCommitmentDoesNotReopen(t *testing.T) {
	for status, want := range map[string]bool{"rejected": false, "deprecated": false, "superseded": false, "accepted": true} {
		g := load(t,
			file("bel_7k3m9q", "status: incorrect"),
			file("task_p4n2xa", "status: "+status, "relies_on: [bel_7k3m9q]"),
		)
		if got := reopens(t, g, "task_p4n2xa", "relies_on"); got != want {
			t.Errorf("a %s task relying on an incorrect belief: reopens = %v", status, got)
		}
	}
}

func TestEdgeReopensNothingElse(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "status: withdrawn"),
		file("bel_c9d4se", "status: held"),
		file("bel_h8c1rt", "status: incorrect"),
		file("dec_9w4hzd", "relies_on: [bel_7k3m9q, bel_c9d4se]"),
		file("dec_p4n2xa", "mentions: [bel_h8c1rt]"),                // doesn't propagate
		file("ev_q2m5vz", "about: [bel_h8c1rt]"),                    // source isn't a commitment
		file("bel_t6w8yb", "answers: [bel_h8c1rt]", "status: held"), // neither
		file("task_k3m9qr", "relies_on: [bel_zzzzzz]"),              // no target
	)
	for _, c := range []struct{ from, name string }{
		{"dec_9w4hzd", "relies_on"}, // the withdrawn belief, then the held one
		{"dec_p4n2xa", "mentions"},
		{"task_k3m9qr", "relies_on"},
	} {
		for _, e := range g.Edges {
			if e.From.ID == c.from && e.Name == c.name && e.Reopens() {
				t.Errorf("%s %s -> %s reopens", c.from, c.name, e.To.Name)
			}
		}
	}
}

// No edge a knowing record holds propagates today, so the test makes one
// that does: only the source's layer then keeps it from reopening.
func TestEdgeDoesNotReopenFromOutsideTheCommittingLayer(t *testing.T) {
	g := load(t,
		file("bel_h8c1rt", "status: incorrect"),
		file("ev_q2m5vz", "about: [bel_h8c1rt]"),
		file("dec_9w4hzd", "mentions: [bel_h8c1rt]"),
	)
	for _, c := range []struct {
		from, field string
		want        bool
	}{
		{"ev_q2m5vz", "about", false},
		{"dec_9w4hzd", "mentions", true},
	} {
		e := edge(t, g, c.from, c.field)
		e.Def.Propagates = true
		if e.Reopens() != c.want {
			t.Errorf("a propagating link from %s: reopens = %v", c.from, !c.want)
		}
	}
}

// A sub-record's link takes its layer from the record that holds it, so a
// propagating link from a knowing record's sub-record never reopens it.
func TestEdgeFromASubRecordDoesNotReopen(t *testing.T) {
	g := load(t,
		file("bel_h8c1rt", "status: incorrect"),
		file("bel_7k3m9q", "status: held", "support:\n  - {id: s1, evidence: [bel_h8c1rt]}"),
	)
	e := edge(t, g, "bel_7k3m9q/s1", "evidence")
	e.Def.Propagates = true
	if e.Reopens() {
		t.Error("a link from a sub-record of a knowing record reopens")
	}
}

// A link to a sub-record is judged by the sub-record's own status, which the
// parent's type reads as fallen or not.
func TestEdgeToASubRecordReopensOnlyWhenItHasFallen(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "status: held", "support:\n  - {id: s1}\n  - {id: s2, status: incorrect}"),
		file("dec_9w4hzd", "relies_on: [bel_7k3m9q/s1]"),
		file("dec_p4n2xa", "relies_on: [bel_7k3m9q/s2]"),
	)
	if reopens(t, g, "dec_9w4hzd", "relies_on") {
		t.Error("a link to a sub-record that holds reopens")
	}
	if !reopens(t, g, "dec_p4n2xa", "relies_on") {
		t.Error("a link to a sub-record that has fallen doesn't reopen")
	}
}

// ---- Node: progress ----

func TestNodeProgress(t *testing.T) {
	g := load(t,
		file("task_p4n2xa"),                          // no progress: the schema's default
		file("task_k3m9qr", "progress: in_progress"), // its own
		file("bel_7k3m9q"),                           // a type with no progress field
	)
	for id, want := range map[string]string{"task_p4n2xa": "todo", "task_k3m9qr": "in_progress", "bel_7k3m9q": ""} {
		if got := g.Node(id).Progress(); got != want {
			t.Errorf("Progress of %s = %q, want %q", id, got, want)
		}
	}
}

func TestNodeProgressOfASubRecord(t *testing.T) {
	g := load(t, file("bel_7k3m9q", "support:\n  - {id: s1}"))
	if got := g.Node("bel_7k3m9q/s1").Progress(); got != "" {
		t.Errorf("Progress of a sub-record = %q", got)
	}
}

// ---- Node and Edge: settling a question ----

// answered loads the records and a question, and returns the question.
func answered(t *testing.T, records ...string) *Node {
	t.Helper()
	g := load(t, append(records, file("q_2wq8dn", "question: Which?"))...)
	return g.Node("q_2wq8dn")
}

func TestNodeSettled(t *testing.T) {
	for _, c := range []struct {
		name    string
		records []string
		want    bool
	}{
		{"nothing answers it", nil, false},
		{"an accepted decision", []string{file("dec_9w4hzd", "status: accepted", "answers: [q_2wq8dn]")}, true},
		{"a held belief", []string{file("bel_7k3m9q", "status: held", "answers: [q_2wq8dn]")}, true},
		{"only a leaning decision", []string{file("dec_9w4hzd", "status: leaning", "answers: [q_2wq8dn]")}, false},
		{"only a proposed decision", []string{file("dec_9w4hzd", "status: proposed", "answers: [q_2wq8dn]")}, false},
		{"a superseded decision", []string{file("dec_9w4hzd", "status: superseded", "answers: [q_2wq8dn]")}, false},
		{"a withdrawn belief", []string{file("bel_7k3m9q", "status: withdrawn", "answers: [q_2wq8dn]")}, false},
	} {
		if got := answered(t, c.records...).Settled(); got != c.want {
			t.Errorf("%s: Settled = %v", c.name, got)
		}
	}
	g := load(t, file("q_2wq8dn", "question: Which?", "dropped: No longer needed"))
	if !g.Node("q_2wq8dn").Settled() {
		t.Error("a dropped question is settled")
	}
}

func TestEdgeSettlesAndPendingPerStatus(t *testing.T) {
	for _, c := range []struct {
		id, status       string
		settles, pending bool
	}{
		{"dec_9w4hzd", "proposed", false, true},
		{"dec_9w4hzd", "leaning", false, true},
		{"dec_9w4hzd", "accepted", true, false},
		{"dec_9w4hzd", "rejected", false, false},
		{"dec_9w4hzd", "deprecated", false, false},
		{"dec_9w4hzd", "superseded", false, false},
		{"bel_7k3m9q", "held", true, false},
		{"bel_7k3m9q", "incorrect", false, false},
		{"bel_7k3m9q", "withdrawn", false, false},
	} {
		g := load(t, file("q_2wq8dn", "question: Which?"), file(c.id, "status: "+c.status, "answers: [q_2wq8dn]"))
		e := edge(t, g, c.id, "answers")
		if e.Settles() != c.settles || e.Pending() != c.pending {
			t.Errorf("%s %s: settles %v, pending %v", c.id, c.status, e.Settles(), e.Pending())
		}
	}
	g := load(t, file("task_p4n2xa", "status: accepted"), file("dec_9w4hzd", "status: accepted", "mentions: [task_p4n2xa]"))
	if e := edge(t, g, "dec_9w4hzd", "mentions"); e.Settles() || e.Pending() {
		t.Error("an edge that lists no statuses settles and pends nothing")
	}
}

// ---- Edge: attributes ----

func TestEdgeAttr(t *testing.T) {
	g := load(t,
		file("who_4rt8kc"),
		file("who_9m2xqd"),
		file("ini_c5d7ab", "slug: s", "purpose: P", "goals: [G]",
			"involves:\n  - {to: who_4rt8kc, role: sponsor}\n  - who_9m2xqd"),
	)
	var got []string
	for _, e := range g.Edges {
		if e.Name == "involves" {
			got = append(got, e.To.Name+"="+e.Attr("role")+"/"+e.Attr("nothing"))
		}
	}
	if want := []string{"who_4rt8kc=sponsor/", "who_9m2xqd=/"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---- Graph: members ----

func TestGraphMembers(t *testing.T) {
	g := load(t,
		file("ini_c5d7ab", "slug: s", "purpose: P", "goals: [G]"),
		file("ini_h8c1rt", "slug: t", "purpose: P", "goals: [G]"),
		file("prd_3vb6ne", "part_of: [ini_c5d7ab]"),
		file("req_m2xqd9", "statement: S", "part_of: [prd_3vb6ne]"), // through the PRD
		file("task_p4n2xa", "part_of: [ini_c5d7ab]"),
		file("plan_k3m9qr", "part_of: [ini_c5d7ab]"),
		file("step_t6w8yb", "part_of: [plan_k3m9qr]"), // through the plan
		file("q_2wq8dn", "question: Which?", "part_of: [ini_c5d7ab]", "options:\n  - {id: o1, title: One}"),
		file("task_zzzzzz", "part_of: [ini_h8c1rt]"), // another initiative's
		file("bel_7k3m9q"), // part of nothing
	)
	want := []string{"plan_k3m9qr", "prd_3vb6ne", "q_2wq8dn", "q_2wq8dn/o1", "req_m2xqd9", "step_t6w8yb", "task_p4n2xa"}
	if got := ids(g.Members(g.Node("ini_c5d7ab"))); !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if got := ids(g.Members(g.Node("ini_h8c1rt"))); !slices.Equal(got, []string{"task_zzzzzz"}) {
		t.Errorf("members of the other = %v", got)
	}
}

func TestGraphProduced(t *testing.T) {
	g := load(t,
		file("ini_c5d7ab", "slug: s", "purpose: P", "goals: [G]"),
		file("ini_h8c1rt", "slug: t", "purpose: P", "goals: [G]"),
		file("actv_a1a1a1", "part_of: [ini_c5d7ab]"),
		file("actv_b2b2b2", "part_of: [ini_c5d7ab]"),
		file("actv_c3c3c3", "part_of: [ini_h8c1rt]"),
		file("fnd_zzzzzz", "generated_by: [actv_a1a1a1]"),
		file("fnd_aaaaaa", "generated_by: [actv_b2b2b2]"),
		file("fnd_qqqqqq", "generated_by: [actv_c3c3c3]"), // another initiative's activity
		file("fnd_nnnnnn"), // generated by nothing
		file("fnd_mmmmmm", "generated_by: [actv_a1a1a1, actv_b2b2b2]"), // two activities, listed once
		file("task_p4n2xa", "part_of: [ini_c5d7ab]"),
	)
	want := []string{"fnd_aaaaaa", "fnd_mmmmmm", "fnd_zzzzzz"}
	if got := ids(g.Produced(g.Node("ini_c5d7ab"))); !slices.Equal(got, want) {
		t.Errorf("produced = %v, want %v", got, want)
	}
	if got := ids(g.Produced(g.Node("ini_h8c1rt"))); !slices.Equal(got, []string{"fnd_qqqqqq"}) {
		t.Errorf("produced by the other = %v", got)
	}
	for _, m := range g.Members(g.Node("ini_c5d7ab")) {
		if strings.HasPrefix(m.ID, "fnd_") {
			t.Errorf("%s is a member", m.ID)
		}
	}
}

func TestGraphMembersEndsOnACycle(t *testing.T) {
	g := load(t,
		file("ini_c5d7ab", "slug: s", "purpose: P", "goals: [G]"),
		file("prd_3vb6ne", "part_of: [ini_c5d7ab]"),
		file("req_m2xqd9", "statement: S", "part_of: [prd_3vb6ne]"),
	)
	// The PRD may only be part of an initiative, so make a cycle by hand.
	req := g.Node("req_m2xqd9")
	req.Incoming = append(req.Incoming, &Edge{From: g.Node("prd_3vb6ne"), Name: "part_of", Target: req})
	want := []string{"prd_3vb6ne", "req_m2xqd9"}
	if got := ids(g.Members(g.Node("ini_c5d7ab"))); !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
}
