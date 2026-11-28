package check

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

func rec(id, title string, lines ...string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\n" + strings.Join(lines, "\n") + "\ncreated: 2026-11-22\n---\n\nBody.\n"
}

// warnings returns the diagnostics of a workspace, which must hold no errors.
func warnings(t *testing.T, records map[string]string) string {
	t.Helper()
	res, got := run(t, testws.New(t, records).Root)
	if errs, _ := res.Diagnostics.Count(); errs != 0 {
		t.Fatalf("errors:\n%s", got)
	}
	var keep []string
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !strings.Contains(l, "run `mason sync`") { // unrelated to the rules here
			keep = append(keep, l+"\n")
		}
	}
	return strings.Join(keep, "")
}

func wantWarning(t *testing.T, name, got, sub string) {
	t.Helper()
	if sub == "" && got != "" {
		t.Errorf("%s: unexpected:\n%s", name, got)
	}
	if sub != "" && !strings.Contains(got, sub) {
		t.Errorf("%s: want %q in:\n%s", name, sub, got)
	}
}

// A belief in force with a disputing support link warns; an incorrect one
// doesn't.
func TestHeldBeliefThatIsDisputedWarns(t *testing.T) {
	belief := func(status string) map[string]string {
		return map[string]string{
			"belief/bel_b1b1b1.md":  rec("bel_b1b1b1", "B", "claim: x", "status: "+status, "asserted_by: agent", "support: [{id: s1, evidence: ev_e1e1e1, direction: disputes}]"),
			"evidence/ev_e1e1e1.md": rec("ev_e1e1e1", "E", "observed_at: 2026-11-22T10:00:00Z", "from: []"),
		}
	}
	got := warnings(t, belief("held"))
	wantWarning(t, "held", got, "a rebut means the belief is incorrect")
	wantWarning(t, "incorrect", warnings(t, belief("incorrect")), "")
}

// The within, target_status and one_in_force constraints each warn when broken.
func TestEdgeConstraintsWarn(t *testing.T) {
	q := func(id string) string { return rec(id, "Q", "question: What?", "options: [{id: o1, title: One}]") }
	dec := func(id, status string, lines ...string) string {
		return rec(id, "D", append([]string{"status: " + status}, lines...)...)
	}
	cases := []struct {
		name    string
		records map[string]string
		want    string
	}{
		{"chooses another question's option", map[string]string{
			"question/q_a1a1a1.md": q("q_a1a1a1"), "question/q_b2b2b2.md": q("q_b2b2b2"),
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "proposed", "answers: [q_a1a1a1]", "chooses: [q_b2b2b2/o1]"),
		}, "a sub-record of q_b2b2b2"},
		{"chooses its own question's option", map[string]string{
			"question/q_a1a1a1.md":   q("q_a1a1a1"),
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "proposed", "answers: [q_a1a1a1]", "chooses: [q_a1a1a1/o1]"),
		}, ""},
		{"two accepted decisions answer one question", map[string]string{
			"question/q_a1a1a1.md":   q("q_a1a1a1"),
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "accepted", "answers: [q_a1a1a1]"),
			"decision/dec_d2d2d2.md": dec("dec_d2d2d2", "accepted", "answers: [q_a1a1a1]"),
		}, "only one record in force should"},
		{"one accepted, one proposed", map[string]string{
			"question/q_a1a1a1.md":   q("q_a1a1a1"),
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "accepted", "answers: [q_a1a1a1]"),
			"decision/dec_d2d2d2.md": dec("dec_d2d2d2", "proposed", "answers: [q_a1a1a1]"),
		}, ""},
		{"supersedes an accepted decision", map[string]string{
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "accepted"),
			"decision/dec_d2d2d2.md": dec("dec_d2d2d2", "accepted", "supersedes: [dec_d1d1d1]"),
		}, "dec_d1d1d1, which is accepted; it should be superseded"},
		{"a proposed decision supersedes an accepted one", map[string]string{
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "accepted"),
			"decision/dec_d2d2d2.md": dec("dec_d2d2d2", "proposed", "supersedes: [dec_d1d1d1]"),
		}, ""},
		{"supersedes a superseded decision", map[string]string{
			"decision/dec_d1d1d1.md": dec("dec_d1d1d1", "superseded"),
			"decision/dec_d2d2d2.md": dec("dec_d2d2d2", "accepted", "supersedes: [dec_d1d1d1]"),
		}, ""},
		{"two plans in force from one task", map[string]string{
			"task/task_t1t1t1.md": rec("task_t1t1t1", "T", "status: accepted"),
			"plan/plan_p1p1p1.md": rec("plan_p1p1p1", "P", "status: accepted", "derived_from: [task_t1t1t1]"),
			"plan/plan_p2p2p2.md": rec("plan_p2p2p2", "P", "status: proposed", "derived_from: [task_t1t1t1]"),
		}, "only one record in force should"},
		{"one plan rejected", map[string]string{
			"task/task_t1t1t1.md": rec("task_t1t1t1", "T", "status: accepted"),
			"plan/plan_p1p1p1.md": rec("plan_p1p1p1", "P", "status: accepted", "derived_from: [task_t1t1t1]"),
			"plan/plan_p2p2p2.md": rec("plan_p2p2p2", "P", "status: rejected", "derived_from: [task_t1t1t1]"),
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { wantWarning(t, c.name, warnings(t, c.records), c.want) })
	}
}
