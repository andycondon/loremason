package graph

import (
	"slices"
	"testing"
)

// recorded is the revisions header of a record whose revision 1 has a
// fingerprint written down, which is when its status starts to seal it.
var recorded = []string{"revisions:", "  - {rev: 1, date: 2026-11-22, fp: \"fp1:000000000000\"}"}

// withRevs adds the recorded revisions to a record's header lines.
func withRevs(lines ...string) []string { return append(slices.Clone(lines), recorded...) }

// A record is sealed by its own status, a step by its plan's, and a question
// by a link that settles it.
func TestSealedByStatus(t *testing.T) {
	g := load(t,
		file("dec_9w4hzd", withRevs("status: accepted")...),
		file("dec_2b4cdf", withRevs("status: proposed")...),
		file("plan_k2x8wd", withRevs("status: accepted")...),
		file("plan_4q7zbd", withRevs("status: proposed")...),
		file("step_2c7rq1", withRevs("part_of: plan_k2x8wd")...),
		file("step_8v0ynf", withRevs("part_of: plan_4q7zbd")...),
		file("q_2wq8dn", withRevs("question: Which?")...),
		file("q_5tk1ab", withRevs("question: Another?")...),
		file("dec_6n1xtb", "status: accepted", "answers: [q_2wq8dn]"),
	)
	for _, c := range []struct {
		id   string
		want []string
	}{
		{"dec_9w4hzd", []string{"accepted"}},
		{"dec_2b4cdf", nil},
		{"step_2c7rq1", []string{"its plan plan_k2x8wd is accepted"}},
		{"step_8v0ynf", nil},
		{"q_2wq8dn", []string{"settled"}},
		{"q_5tk1ab", nil},
	} {
		n := g.Node(c.id)
		if got := n.SealedBy(1); !slices.Equal(got, c.want) || n.Sealed(1) != (c.want != nil) {
			t.Errorf("%s: SealedBy(1) = %v, want %v", c.id, got, c.want)
		}
	}
}

// Status seals only the current revision; older ones stay as they were.
func TestSealedByStatusIsForTheCurrentRevision(t *testing.T) {
	g := load(t, file("dec_9w4hzd", "status: accepted", "revisions:", "  - {rev: 1, fp: \"fp1:000000000000\"}", "  - {rev: 2, fp: \"fp1:000000000001\"}"))
	n := g.Node("dec_9w4hzd")
	if n.Sealed(1) || !n.Sealed(2) {
		t.Errorf("sealed: rev 1 %v, rev 2 %v", n.Sealed(1), n.Sealed(2))
	}
}

// A status seals a revision only once its fingerprint is recorded: a revision
// just opened by `mason revise` can be edited until `mason sync` writes it.
func TestStatusDoesNotSealARevisionWithNoFingerprint(t *testing.T) {
	g := load(t,
		file("dec_9w4hzd", "status: accepted"), // implicit revision 1
		file("dec_2b4cdf", "status: accepted", "revisions:", "  - {rev: 1, fp: \"fp1:000000000000\"}", "  - {rev: 2, breaking: true}"),
	)
	for _, id := range []string{"dec_9w4hzd", "dec_2b4cdf"} {
		n := g.Node(id)
		if n.Sealed(n.Current().Rev) {
			t.Errorf("%s: sealed by %v with no recorded fingerprint", id, n.SealedBy(n.Current().Rev))
		}
		if !n.SealedByStatus() || len(n.stateSealStrings()) == 0 {
			t.Errorf("%s: its status should still be reported as the reason it will seal", id)
		}
	}
}

// A cycle of status_from edges has no status holder, instead of looping: the
// record is in force and sealed by nothing.
func TestStatusHolderSurvivesACycle(t *testing.T) {
	g := load(t,
		file("step_2c7rq1", withRevs("follows: step_8v0ynf")...),
		file("step_8v0ynf", withRevs("follows: step_2c7rq1")...),
		file("step_4b9dxc", withRevs("part_of: plan_k2x8wd")...),
		file("plan_k2x8wd", withRevs("status: accepted")...),
	)
	step := g.Node("step_2c7rq1").Type()
	step.StatusFrom = "follows"
	defer func() { step.StatusFrom = "part_of" }()

	a := g.Node("step_2c7rq1")
	if a.statusHolder() != nil {
		t.Errorf("statusHolder = %v, want nil on a cycle", a.statusHolder())
	}
	if !a.InForce() || a.SealedByStatus() || len(a.stateSealStrings()) != 0 {
		t.Errorf("a cycle: in force %v, sealed by status %v, seals %v", a.InForce(), a.SealedByStatus(), a.stateSealStrings())
	}
	step.StatusFrom = "part_of"
	if h := g.Node("step_4b9dxc").statusHolder(); h == nil || h.ID != "plan_k2x8wd" {
		t.Errorf("a step's status holder = %v, want its plan", h)
	}
}

// Seals splits a revision's seals into those in force and those pending: a
// record's state seals the current revision only once its fingerprint is
// recorded, and a question's seal is its answer, not a status.
func TestSeals(t *testing.T) {
	g := load(t,
		file("dec_9w4hzd", withRevs("status: accepted")...),
		file("dec_2b4cdf", "status: accepted"), // implicit revision 1: pending
		file("q_2wq8dn", "question: Which?"),
		file("dec_6n1xtb", "status: accepted", "answers: [q_2wq8dn]"),
	)
	for _, c := range []struct {
		id      string
		active  []string
		pending []string
	}{
		{"dec_9w4hzd", []string{"accepted"}, nil},
		{"dec_2b4cdf", nil, []string{"accepted"}},
		{"q_2wq8dn", nil, []string{"settled"}},
	} {
		active, pending := g.Node(c.id).Seals(1)
		if !slices.Equal(sealStrings(active), c.active) || !slices.Equal(sealStrings(pending), c.pending) {
			t.Errorf("%s: active %v, pending %v; want %v, %v", c.id, active, pending, c.active, c.pending)
		}
	}
	if _, pending := g.Node("q_2wq8dn").Seals(1); len(pending) != 1 || !pending[0].Answered {
		t.Errorf("a question's pending seal should be its answer: %v", pending)
	}
}
