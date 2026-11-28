package graph

import "testing"

// A quote's pin records a revision and seals nothing; a propagating pin
// seals.
func TestQuotePinDoesNotSeal(t *testing.T) {
	pin := func(to string) string {
		return "  - {to: " + to + ", rev: 1, fp: \"fp1:000000000000\"}"
	}
	g := load(t,
		file("fnd_k3m9qx", "revisions:", "  - {rev: 1}", "  - {rev: 2}"),
		file("ev_p4n2xa", "from:", pin("fnd_k3m9qx")),
		file("dd_c5r1tw", "status: proposed", "revisions:", "  - {rev: 1}", "  - {rev: 2}"),
		file("task_9w4hzd", "status: accepted", "derived_from:", pin("dd_c5r1tw")),
	)
	quote := edge(t, g, "ev_p4n2xa", "from")
	if quote.Pin == nil || quote.Seals() {
		t.Errorf("a quote is pinned but doesn't seal: pin %v, seals %v", quote.Pin, quote.Seals())
	}
	if quote.Stale() {
		t.Error("a quote of an older revision isn't stale")
	}
	if fnd := g.Node("fnd_k3m9qx"); fnd.Sealed(1) || fnd.SealedBy(1) != nil {
		t.Error("a quoted finding's revision isn't sealed or pinned by the quote")
	}

	built := edge(t, g, "task_9w4hzd", "derived_from")
	if !built.Seals() || !built.Stale() {
		t.Errorf("a pin on a propagating edge seals, and is stale behind: seals %v, stale %v", built.Seals(), built.Stale())
	}
	if dd := g.Node("dd_c5r1tw"); !dd.Sealed(1) || len(dd.SealedBy(1)) != 1 {
		t.Error("a design doc's pinned revision is sealed")
	}
}
