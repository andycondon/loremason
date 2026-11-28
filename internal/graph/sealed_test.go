package graph

import (
	"slices"
	"testing"
)

func TestSealedByStatus(t *testing.T) {
	g := load(t,
		file("dec_9w4hzd", "status: accepted"),
		file("dec_2b4cdf", "status: proposed"),
		file("plan_k2x8wd", "status: accepted"),
		file("plan_4q7zbd", "status: proposed"),
		file("step_2c7rq1", "part_of: plan_k2x8wd"),
		file("step_8v0ynf", "part_of: plan_4q7zbd"),
		file("q_2wq8dn", "question: Which?"),
		file("q_5tk1ab", "question: Another?"),
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
	g := load(t, file("dec_9w4hzd", "status: accepted", "revisions:", "  - {rev: 1}", "  - {rev: 2}"))
	n := g.Node("dec_9w4hzd")
	if n.Sealed(1) || !n.Sealed(2) {
		t.Errorf("sealed: rev 1 %v, rev 2 %v", n.Sealed(1), n.Sealed(2))
	}
}
