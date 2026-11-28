package graph

import (
	"fmt"
	"testing"
	"time"
)

// ---- InForce, Orphaned, Doubted, reliesOnDoubted ----

func ev(id, observed string, lines ...string) string {
	return file(id, append([]string{"observed_at: " + observed, "from: []"}, lines...)...)
}

// InForce is false for closed and fallen statuses, and for a step whose plans
// are all out of force.
func TestInForce(t *testing.T) {
	g := load(t,
		file("bel_a1a1a1", "status: held"),
		file("bel_b2b2b2", "status: withdrawn"),
		file("bel_c3c3c3", "status: incorrect"),
		file("bel_d4d4d4", "status: insufficient"),
		file("task_e5e5e5", "status: accepted"),
		file("task_f6f6f6", "status: rejected"),
		file("task_g7g7g7", "status: superseded"),
		file("plan_h8h8h8", "status: accepted"),
		file("plan_j9j9j9", "status: superseded"),
		file("step_m1m1m1", "part_of: plan_h8h8h8"),
		file("step_k2k2k2", "part_of: plan_j9j9j9"),
		file("q_n4n4n4", "question: Which?"),
	)
	for id, want := range map[string]bool{
		"bel_a1a1a1": true, "bel_b2b2b2": false, "bel_c3c3c3": false, "bel_d4d4d4": false,
		"task_e5e5e5": true, "task_f6f6f6": false, "task_g7g7g7": false,
		"plan_h8h8h8": true, "plan_j9j9j9": false,
		"step_m1m1m1": true, "step_k2k2k2": false,
		"q_n4n4n4": true,
	} {
		if got := g.Node(id).InForce(); got != want {
			t.Errorf("%s: InForce = %v, want %v", id, got, want)
		}
	}
}

// Orphaned needs a propagating link from a record in force to one that isn't,
// and leaves a fallen target to Reopens.
func TestOrphaned(t *testing.T) {
	g := load(t,
		file("bel_a1a1a1", "status: held"),
		file("bel_b2b2b2", "status: withdrawn"),
		file("bel_c3c3c3", "status: incorrect"), // fallen: reopens instead
		file("task_t1t1t1", "status: superseded"),
		file("dec_d1d1d1", "status: accepted", "relies_on: [bel_a1a1a1, bel_b2b2b2, bel_c3c3c3]"),
		file("dec_d2d2d2", "status: rejected", "relies_on: [bel_b2b2b2]"),
		file("plan_p2p2p2", "status: accepted", "derived_from: [task_t1t1t1]"),
	)
	for _, c := range []struct {
		from, name, to string
		want           bool
	}{
		{"dec_d1d1d1", "relies_on", "bel_a1a1a1", false},
		{"dec_d1d1d1", "relies_on", "bel_b2b2b2", true},
		{"dec_d1d1d1", "relies_on", "bel_c3c3c3", false},
		{"dec_d2d2d2", "relies_on", "bel_b2b2b2", false},
		{"plan_p2p2p2", "derived_from", "task_t1t1t1", true},
	} {
		var got, found bool
		for _, e := range g.Edges {
			if e.From.ID == c.from && e.Name == c.name && e.Target != nil && e.Target.ID == c.to {
				got, found = e.Concerns().Has(Orphaned), true
			}
		}
		if !found || got != c.want {
			t.Errorf("%s.%s -> %s: Orphaned = %v (found %v), want %v", c.from, c.name, c.to, got, found, c.want)
		}
	}
}

// A step of a superseded plan has no status of its own: it is out of force
// with its plan, so its link to that plan isn't reported as orphaned.
func TestStepsOfASupersededPlanAreNotReportedOrphaned(t *testing.T) {
	g := load(t,
		file("plan_p1p1p1", "status: superseded"),
		file("plan_p2p2p2", "status: accepted"),
		file("step_s1s1s1", "part_of: plan_p1p1p1"),
		file("step_s2s2s2", "part_of: plan_p2p2p2"),
	)
	if edge(t, g, "step_s1s1s1", "part_of").Concerns().Has(Orphaned) {
		t.Error("a step of a superseded plan is out of force, so isn't orphaned")
	}
	if edge(t, g, "step_s2s2s2", "part_of").Concerns().Has(Orphaned) {
		t.Error("a step of a plan in force isn't orphaned")
	}
}

// doubtGraph builds a decision relying on a belief supported by ev_e1e1e1.
// extra adds records; via is written into the decision's pin when not empty.
func doubtGraph(t *testing.T, direction, via string, extra ...string) *Graph {
	t.Helper()
	pin := "bel_b1b1b1"
	if via != "" {
		pin = "{to: bel_b1b1b1, rev: 1, fp: \"fp1:000000000000\", via: " + via + "}"
	}
	records := []string{
		ev("ev_e1e1e1", "2026-11-22T10:00:00Z"),
		file("bel_b1b1b1", "status: held", fmt.Sprintf("support: [{id: s1, evidence: ev_e1e1e1, direction: %s}]", direction)),
		file("dec_d1d1d1", "status: accepted", "relies_on: ["+pin+"]"),
	}
	return load(t, append(records, extra...)...)
}

func doubts(t *testing.T, g *Graph) bool {
	return edge(t, g, "dec_d1d1d1", "relies_on").Concerns().Has(Doubted)
}

// A belief is doubted by evidence undermining or undercutting a supporting
// link, but not a disputing one.
func TestDoubted(t *testing.T) {
	undermine := ev("ev_e2e2e2", "2026-11-23T10:00:00Z", "undermines: [ev_e1e1e1]")
	undercut := ev("ev_e2e2e2", "2026-11-23T10:00:00Z", "undercuts: [bel_b1b1b1/s1]")
	for _, c := range []struct {
		name      string
		direction string
		extra     []string
		want      bool
	}{
		{"nothing against it", "supports", nil, false},
		{"undermined evidence", "supports", []string{undermine}, true},
		{"undercut link", "supports", []string{undercut}, true},
		{"undermined evidence of a disputing link", "disputes", []string{undermine}, false},
		{"undercut disputing link", "disputes", []string{undercut}, false},
	} {
		g := doubtGraph(t, c.direction, "", c.extra...)
		if got := g.Node("bel_b1b1b1").Doubted(); got != c.want {
			t.Errorf("%s: Doubted = %v, want %v", c.name, got, c.want)
		}
		if got := doubts(t, g); got != c.want {
			t.Errorf("%s: doubts = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestADoubtedBeliefThatHasFallenIsSettled(t *testing.T) {
	g := load(t,
		ev("ev_e1e1e1", "2026-11-22T10:00:00Z"),
		ev("ev_e2e2e2", "2026-11-23T10:00:00Z", "undermines: [ev_e1e1e1]"),
		file("bel_b1b1b1", "status: insufficient", "support: [{id: s1, evidence: ev_e1e1e1, direction: supports}]"),
		file("dec_d1d1d1", "status: accepted", "relies_on: [bel_b1b1b1]"),
	)
	if g.Node("bel_b1b1b1").Doubted() || doubts(t, g) {
		t.Error("a belief that has fallen is settled, not doubted")
	}
}

func TestDoubtsOnlyForACommitmentInForce(t *testing.T) {
	g := load(t,
		ev("ev_e1e1e1", "2026-11-22T10:00:00Z"),
		ev("ev_e2e2e2", "2026-11-23T10:00:00Z", "undermines: [ev_e1e1e1]"),
		file("bel_b1b1b1", "status: held", "support: [{id: s1, evidence: ev_e1e1e1, direction: supports}]"),
		file("dec_d1d1d1", "status: rejected", "relies_on: [bel_b1b1b1]"),
		file("task_d2d2d2", "status: accepted", "blocked_by: [bel_b1b1b1]"),
	)
	if doubts(t, g) || edge(t, g, "task_d2d2d2", "blocked_by").Concerns().Has(Doubted) {
		t.Error("a closed source, or a link that doesn't propagate, isn't doubted")
	}
}

// A re-check records evidence in the pin's via. It clears the doubt when that
// evidence was observed no earlier than all of the evidence against the
// belief; newer evidence against it doubts it again.
func TestRecheckClearsADoubtUntilNewerEvidenceArrives(t *testing.T) {
	undermine := ev("ev_e2e2e2", "2026-11-23T10:00:00Z", "undermines: [ev_e1e1e1]")
	older := ev("ev_e3e3e3", "2026-11-22T12:00:00Z", "about: [bel_b1b1b1]")
	newer := ev("ev_e4e4e4", "2026-11-24T10:00:00Z", "about: [bel_b1b1b1]")
	newerUndermine := ev("ev_e5e5e5", "2026-11-25T10:00:00Z", "undermines: [ev_e1e1e1]")
	same := ev("ev_e0e0e0", "2026-11-23T10:00:00Z", "about: [bel_b1b1b1]") // same time, earlier ID

	undermineAt := func(when string) string { return ev("ev_e2e2e2", when, "undermines: [ev_e1e1e1]") }
	at := func(id, when string) string { return ev(id, when, "about: [bel_b1b1b1]") }

	for _, c := range []struct {
		name  string
		via   string
		extra []string
		want  bool
	}{
		{"doubted, no re-check", "", []string{undermine}, true},
		{"re-checked with older evidence", "ev_e3e3e3", []string{undermine, older}, true},
		{"re-checked with newer evidence", "ev_e4e4e4", []string{undermine, newer}, false},
		{"newer undermine after the re-check", "ev_e4e4e4", []string{undermine, newer, newerUndermine}, true},
		{"re-checked by the evidence itself", "ev_e2e2e2", []string{undermine}, false},
		{"same time clears, whatever the IDs", "ev_e0e0e0", []string{undermine, same}, false},
		{"offset before: 12:00+05:00 is 07:00Z", "ev_e3e3e3", []string{undermineAt("2026-11-23T08:00:00Z"), at("ev_e3e3e3", "2026-11-23T12:00:00+05:00")}, true},
		{"offset after: 12:00+02:00 is 10:00Z", "ev_e3e3e3", []string{undermineAt("2026-11-23T08:00:00Z"), at("ev_e3e3e3", "2026-11-23T12:00:00+02:00")}, false},
		{"dates tie and clear", "ev_e3e3e3", []string{undermineAt("2026-11-23"), at("ev_e3e3e3", "2026-11-23")}, false},
		{"an earlier date doesn't clear", "ev_e3e3e3", []string{undermineAt("2026-11-23"), at("ev_e3e3e3", "2026-11-22")}, true},
		{"unparsable observed_at clears nothing", "ev_e3e3e3", []string{undermine, at("ev_e3e3e3", "yesterday")}, true},
		{"re-checked and nothing against it", "ev_e4e4e4", []string{newer}, false},
	} {
		g := doubtGraph(t, "supports", c.via, c.extra...)
		if got := doubts(t, g); got != c.want {
			t.Errorf("%s: doubts = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestObservedAt(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string // RFC 3339 in UTC, or "" when it can't be read
	}{
		{"2026-11-22", "2026-11-22T00:00:00Z"},
		{"2026-11-22T14:30:00Z", "2026-11-22T14:30:00Z"},
		{"2026-11-22t14:30z", "2026-11-22T14:30:00Z"},
		{"2026-11-22 14:30:00 +05:00", "2026-11-22T09:30:00Z"},
		{"2026-11-22T14:30:00.5", "2026-11-22T14:30:00Z"},
		{"", ""},
		{"soon", ""},
	} {
		g := load(t, ev("ev_e1e1e1", c.in))
		got, ok := g.Node("ev_e1e1e1").observedAt()
		switch {
		case c.want == "" && ok:
			t.Errorf("%q: read as %v, want unreadable", c.in, got)
		case c.want != "" && (!ok || got.UTC().Truncate(time.Second).Format(time.RFC3339) != c.want):
			t.Errorf("%q: got %v (%v), want %s", c.in, got, ok, c.want)
		}
	}
}

// One guard for every concern: a link from a record that isn't in force has
// none, even a pin that is behind its target.
func TestConcernsIgnoreASourceNotInForce(t *testing.T) {
	g := load(t,
		file("bel_b1b1b1", "status: withdrawn"),
		file("dec_d1d1d1", "status: accepted", "relies_on: [bel_b1b1b1]"),
		file("dec_d2d2d2", "status: rejected", "relies_on: [bel_b1b1b1]"),
		file("task_d3d3d3", "status: accepted", "blocked_by: [bel_b1b1b1]"),
		file("bel_b2b2b2", "status: held"),
		file("dec_d4d4d4", "status: accepted", `relies_on: [{to: bel_b2b2b2, rev: 1, fp: "fp1:000000000000"}]`),
		file("dec_d5d5d5", "status: rejected", `relies_on: [{to: bel_b2b2b2, rev: 1, fp: "fp1:000000000000"}]`),
	)
	if got := edge(t, g, "dec_d4d4d4", "relies_on").Concerns(); got != Stale {
		t.Errorf("accepted source, stale pin: Concerns = %q, want stale", got)
	}
	if got := edge(t, g, "dec_d5d5d5", "relies_on").Concerns(); got.Has(Stale) {
		t.Errorf("rejected source, stale pin: Concerns = %q, want no stale", got)
	}
	if got := edge(t, g, "dec_d1d1d1", "relies_on").Concerns(); got != Orphaned {
		t.Errorf("accepted source: Concerns = %q, want orphaned", got)
	}
	if got := edge(t, g, "dec_d2d2d2", "relies_on").Concerns(); got != 0 {
		t.Errorf("rejected source: Concerns = %q, want none", got)
	}
	if got := edge(t, g, "task_d3d3d3", "blocked_by").Concerns(); got != 0 {
		t.Errorf("link that doesn't propagate: Concerns = %q, want none", got)
	}
}
