package index

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// ---- records ----

const (
	iniA = "ini_a1b2c3"
	iniB = "ini_d4e5f6"
)

// initiative is an initiative record with a slug, an alias, and the lines
// given.
func initiative(id, slug string, lines ...string) string {
	return record(id, "Initiative "+slug, append([]string{"slug: " + slug, "status: active", "aliases: [" + slug + "-alias]"}, lines...)...)
}

// question is a question that is part of an initiative.
func question(id, ini string, lines ...string) string {
	return record(id, "Question "+id, append([]string{"question: What about " + id + "?", "part_of: " + ini}, lines...)...)
}

// decision is a decision answering a question.
func decision(id, ini, status, answers string) string {
	return record(id, "Decision "+id, "status: "+status, "part_of: "+ini, "answers: ["+answers+"]")
}

// task is a task that is part of an initiative.
func task(id, title, ini string, lines ...string) string {
	return record(id, title, append([]string{"status: accepted", "part_of: " + ini}, lines...)...)
}

// first is the wording of a belief's first revision.
func first(id string) string { return "the first claim of " + id }

// firstFP is the fingerprint of a belief's first revision.
func firstFP(id string) string {
	return fingerprint.Of(map[string]any{"claim": first(id)}, "", false).String()
}

// belief is a belief with a second revision, so a link pinned to the first
// is stale.
func belief(id, status string) string {
	return `---
id: ` + id + `
title: Belief ` + id + `
claim: the second claim of ` + id + `
status: ` + status + `
asserted_by: agent
revisions:
  - {rev: 1, date: 2026-11-22, fp: "` + firstFP(id) + `", claim: ` + first(id) + `}
  - {rev: 2, date: 2026-11-23, fp: "fp1:000000000002", change: breaking, claim: the second claim of ` + id + `}
created: 2026-11-22
---
`
}

// pinned is a relies_on line pinned to the first revision of a belief.
func pinned(belief string) string {
	return `relies_on: [{to: ` + belief + `, rev: 1, fp: "` + firstFP(belief) + `"}]`
}

// where returns the view of an initiative.
func where(t *testing.T, x *Index, name string) *View {
	t.Helper()
	v, err := x.Where(name)
	if err != nil {
		t.Fatalf("Where(%s): %v", name, err)
	}
	return v
}

// ---- Where: progress ----

func TestWhereFollowsAnEditWithoutSync(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "First", iniA, "progress: todo"),
	})
	x := open(t, w)
	if got := where(t, x, "alpha").InProgress; len(got) != 0 {
		t.Fatalf("nothing is in progress yet: %+v", got)
	}
	x.Close()

	edit(t, w, "task/task_t1t1t1.md", "progress: todo", "progress: in_progress")
	x = open(t, w)
	if !x.Rebuilt() {
		t.Error("the edit should make the open rebuild")
	}
	if got := ids(where(t, x, "alpha").InProgress); got != "task_t1t1t1" {
		t.Errorf("in progress = %q, want task_t1t1t1", got)
	}
}

func TestWhereListsInProgressAndBlockedWithBlockers(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
		"question/q_c1c1c1.md":       question("q_c1c1c1", iniA),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "No progress field", iniA),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Working", iniA, "progress: in_progress"),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "Stuck", iniA, "progress: blocked", "blocked_by: [task_t5t5t5, q_c1c1c1]"),
		"task/task_t4t4t4.md":        task("task_t4t4t4", "Done", iniA, "progress: done"),
		"task/task_t5t5t5.md":        task("task_t5t5t5", "Stuck without a reason", iniA, "progress: blocked"),
		"task/task_t6t6t6.md":        task("task_t6t6t6", "Another initiative's", iniB, "progress: in_progress"),
	})
	valid(t, w)
	v := where(t, open(t, w), "alpha")
	if got := ids(v.InProgress); got != "task_t2t2t2" {
		t.Errorf("in progress = %q, want task_t2t2t2", got)
	}
	if len(v.Blocked) != 2 {
		t.Fatalf("blocked = %+v, want two", v.Blocked)
	}
	b := v.Blocked[0]
	if b.Task.ID != "task_t3t3t3" || ids(b.By) != "q_c1c1c1 task_t5t5t5" {
		t.Errorf("first blocked = %+v, want task_t3t3t3 by q_c1c1c1 and task_t5t5t5", b)
	}
	if b := v.Blocked[1]; b.Task.ID != "task_t5t5t5" || len(b.By) != 0 {
		t.Errorf("second blocked = %+v, want task_t5t5t5 with no blockers", b)
	}
	if v.Slug != "alpha" || v.Initiative.ID != iniA || v.Initiative.Type != "initiative" {
		t.Errorf("initiative = %+v, slug %q", v.Initiative, v.Slug)
	}
}

// ---- Where: open questions ----

// questionsWS has the five cases of open questions, in initiative alpha:
// q1 has nothing, q2 an accepted decision, q3 a held belief, q4 a leaning
// decision, q5 is dropped. A proposed decision also waits on q1, and a
// rejected one on q4.
func questionsWS() map[string]string {
	return map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"question/q_c1c1c1.md":       question("q_c1c1c1", iniA),
		"question/q_c2c2c2.md":       question("q_c2c2c2", iniA),
		"question/q_c3c3c3.md":       question("q_c3c3c3", iniA),
		"question/q_c4c4c4.md":       question("q_c4c4c4", iniA),
		"question/q_c5c5c5.md":       question("q_c5c5c5", iniA, "dropped: no longer matters"),
		"decision/dec_d2d2d2.md":     decision("dec_d2d2d2", iniA, "accepted", "q_c2c2c2"),
		"decision/dec_d4d4d4.md":     decision("dec_d4d4d4", iniA, "leaning", "q_c4c4c4"),
		"decision/dec_d5d5d5.md":     decision("dec_d5d5d5", iniA, "proposed", "q_c1c1c1"),
		"decision/dec_d6d6d6.md":     decision("dec_d6d6d6", iniA, "rejected", "q_c4c4c4"),
		"decision/dec_d8d8d8.md":     decision("dec_d8d8d8", iniA, "proposed", "q_c5c5c5"),
		"belief/bel_b3b3b3.md":       strings.NewReplacer("bel_7k3m9q", "bel_b3b3b3", "status: held", "status: held\nanswers: [q_c3c3c3]").Replace(testws.Belief),
	}
}

func TestWhereListsTheOpenQuestions(t *testing.T) {
	w := newWS(t, questionsWS())
	valid(t, w)
	v := where(t, open(t, w), "alpha")

	var got []string
	for _, q := range v.Open {
		got = append(got, q.ID+"<"+ids(q.Pending)+">")
	}
	want := []string{"q_c1c1c1<dec_d5d5d5>", "q_c4c4c4<dec_d4d4d4>"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("open = %v, want %v (q2, q3 and q5 are settled or dropped; a rejected decision isn't shown)", got, want)
	}
	if len(v.Open) == 2 && v.Open[1].Pending[0].Status != "leaning" {
		t.Errorf("q4's pending answer = %+v, want the leaning decision", v.Open[1].Pending[0])
	}
}

func TestSupersedingTheDecisionReopensTheQuestion(t *testing.T) {
	records := questionsWS()
	records["decision/dec_d7d7d7.md"] = record("dec_d7d7d7", "Replacement", "status: accepted", "part_of: "+iniA, "supersedes: [dec_d2d2d2]")
	w := newWS(t, records)
	view := func() *View { return where(t, open(t, w), "alpha") }
	for _, q := range view().Open {
		if q.ID == "q_c2c2c2" {
			t.Fatal("q2 is answered by an accepted decision, so it should not be open")
		}
	}
	edit(t, w, "decision/dec_d2d2d2.md", "status: accepted", "status: superseded")
	var found bool
	for _, q := range view().Open {
		found = found || q.ID == "q_c2c2c2"
	}
	if !found {
		t.Error("q2 should be open again once its decision is superseded by one that doesn't answer it")
	}
}

// ---- Where: membership and re-checks ----

func TestWhereFollowsPartOfThroughPRDAndPlan(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"belief/bel_b1b1b1.md":       belief("bel_b1b1b1", "held"),
		"prd/prd_p1p1p1.md":          record("prd_p1p1p1", "A PRD", "status: accepted", "part_of: "+iniA),
		"requirement/req_r1r1r1.md": `---
id: req_r1r1r1
title: A requirement
statement: do the thing
status: accepted
part_of: prd_p1p1p1
` + pinned("bel_b1b1b1") + `
created: 2026-11-22
---
`,
		"plan/plan_p2p2p2.md": record("plan_p2p2p2", "A plan", "status: accepted", "part_of: "+iniA),
		"step/step_s2s2s2.md": record("step_s2s2s2", "The second step", "part_of: plan_p2p2p2"),
		"step/step_s1s1s1.md": record("step_s1s1s1", "The first step", `part_of: [{to: plan_p2p2p2, rev: 1, fp: "fp1:0000000000a1"}]`),
		"task/task_t9t9t9.md": record("task_t9t9t9", "Outside", "status: accepted", pinned("bel_b1b1b1")),
	})
	valid(t, w)
	v := where(t, open(t, w), "alpha")
	var got []string
	for _, c := range v.Recheck {
		for _, l := range c.Links {
			got = append(got, c.Target.ID+" <- "+l.Other.ID+"."+l.Field)
		}
	}
	want := []string{"bel_b1b1b1 <- req_r1r1r1.relies_on", "plan_p2p2p2 <- step_s1s1s1.part_of"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recheck = %v, want %v (a task that is part of no initiative isn't listed)", got, want)
	}
}

func TestRechecksAreScopedToEachInitiativesOwnRecords(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
		"belief/bel_b1b1b1.md":       belief("bel_b1b1b1", "held"),
		"belief/bel_b2b2b2.md":       belief("bel_b2b2b2", "held"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Alpha's", iniA, pinned("bel_b1b1b1")),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Beta's", iniB, pinned("bel_b1b1b1")),
	})
	valid(t, w)
	x := open(t, w)
	for _, c := range []struct{ ini, task string }{{"alpha", "task_t1t1t1"}, {"beta", "task_t2t2t2"}} {
		v := where(t, x, c.ini)
		if len(v.Recheck) != 1 {
			t.Fatalf("%s: recheck = %+v, want one change", c.ini, v.Recheck)
		}
		ch := v.Recheck[0]
		if ch.Target.ID != "bel_b1b1b1" || ch.Revision != 2 || len(ch.Links) != 1 {
			t.Fatalf("%s: change = %+v", c.ini, ch)
		}
		l := ch.Links[0]
		if l.Other.ID != c.task || l.Field != "relies_on" || !l.Stale || l.Reopens || l.Pin == nil || l.Pin.Rev != 1 {
			t.Errorf("%s: link = %+v", c.ini, l)
		}
		for _, other := range v.Recheck {
			if other.Target.ID == "bel_b2b2b2" {
				t.Errorf("%s lists a belief nothing relies on", c.ini)
			}
		}
	}
}

func TestRecheckShowsALinkThatReopens(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"belief/bel_b1b1b1.md":       belief("bel_b1b1b1", "incorrect"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Relies", iniA, "relies_on: [bel_b1b1b1]"),
	})
	valid(t, w)
	v := where(t, open(t, w), "alpha")
	if len(v.Recheck) != 1 || len(v.Recheck[0].Links) != 1 {
		t.Fatalf("recheck = %+v", v.Recheck)
	}
	if l := v.Recheck[0].Links[0]; !l.Reopens || l.Stale {
		t.Errorf("link = %+v, want reopens and not stale", l)
	}
}

func TestEveryListInTheViewIsSortedByID(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"belief/bel_b1b1b1.md":       belief("bel_b1b1b1", "held"),
		"belief/bel_b2b2b2.md":       belief("bel_b2b2b2", "held"),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "Z", iniA, "progress: in_progress", pinned("bel_b2b2b2")),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Y", iniA, "progress: in_progress", pinned("bel_b2b2b2")),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "X", iniA, "progress: in_progress", pinned("bel_b1b1b1")),
	})
	v := where(t, open(t, w), "alpha")
	if got := ids(v.InProgress); got != "task_t1t1t1 task_t2t2t2 task_t3t3t3" {
		t.Errorf("in progress = %q", got)
	}
	if len(v.Recheck) != 2 || v.Recheck[0].Target.ID != "bel_b1b1b1" || v.Recheck[1].Target.ID != "bel_b2b2b2" {
		t.Fatalf("recheck = %+v", v.Recheck)
	}
	if l := v.Recheck[1].Links; len(l) != 2 || l[0].Other.ID != "task_t1t1t1" || l[1].Other.ID != "task_t3t3t3" {
		t.Errorf("links = %+v, want sorted by source", l)
	}
}

// ---- lookup ----

func TestWhereTakesASlugAnIDOrAnAlias(t *testing.T) {
	w := newWS(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	x := open(t, w)
	for _, name := range []string{"alpha", iniA, "alpha-alias"} {
		if v := where(t, x, name); v.Initiative.ID != iniA || v.Slug != "alpha" {
			t.Errorf("Where(%s) = %+v", name, v)
		}
	}
}

func TestWhereSuggestsTheNearestSlugOrID(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "robust-validate"),
		"initiative/" + iniB + ".md": initiative(iniB, "generated-pages"),
	})
	x := open(t, w)
	for name, want := range map[string]string{"robust-validat": "robust-validate", "ini_a1b2c4": iniA} {
		_, err := x.Where(name)
		var nf *NotFoundError
		if !errors.Is(err, ErrNotFound) || !errors.As(err, &nf) {
			t.Fatalf("Where(%s) = %v, want a not found error", name, err)
		}
		if len(nf.Close) == 0 || nf.Close[0] != want {
			t.Errorf("Where(%s) suggests %v, want %s first", name, nf.Close, want)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q doesn't name %s", err, want)
		}
	}
}

func TestWhereRefusesARecordThatIsNotAnInitiative(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "A task", iniA),
		"decision/dec_d2d2d2.md":     decision("dec_d2d2d2", iniA, "accepted", ""),
	})
	x := open(t, w)
	for id, want := range map[string]string{
		"task_t1t1t1": "task_t1t1t1 is a task, not an initiative",
		"dec_d2d2d2":  "dec_d2d2d2 is a decision, not an initiative",
	} {
		_, err := x.Where(id)
		var ni *NotInitiativeError
		if !errors.As(err, &ni) || err.Error() != want {
			t.Errorf("Where(%s) = %v, want %q", id, err, want)
		}
	}
}

func TestASharedSlugIsAnErrorNamingBoth(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "same"),
		"initiative/" + iniB + ".md": initiative(iniB, "same"),
	})
	x := open(t, w)
	for name, call := range map[string]func() error{
		"Where":    func() error { _, err := x.Where("same"); return err },
		"Overview": func() error { _, err := x.Overview("same"); return err },
	} {
		err := call()
		var d *DuplicateSlugError
		if !errors.As(err, &d) || !strings.Contains(err.Error(), iniA) || !strings.Contains(err.Error(), iniB) {
			t.Errorf("%s = %v, want a DuplicateSlugError naming %s and %s", name, err, iniA, iniB)
		}
	}
	if v := where(t, x, iniB); v.Initiative.ID != iniB {
		t.Errorf("the ID still names one: %+v", v)
	}
}

func TestInitiativesAreListedByID(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "A task", iniA),
	})
	got, err := open(t, w).Initiatives()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != iniA || got[1].ID != iniB || got[0].Type != "initiative" || got[0].Title != "Initiative alpha" {
		t.Fatalf("Initiatives = %+v", got)
	}
	if got[0].Slug != "alpha" || got[1].Slug != "beta" {
		t.Errorf("slugs = %q, %q", got[0].Slug, got[1].Slug)
	}
}

func TestAnInitiativeWithNoSlugIsNotFoundByAnEmptyName(t *testing.T) {
	w := newWS(t, map[string]string{"initiative/" + iniA + ".md": record(iniA, "No slug", "status: active")})
	x := open(t, w)
	for name, call := range map[string]func() error{
		"Where":    func() error { _, err := x.Where(""); return err },
		"Overview": func() error { _, err := x.Overview(""); return err },
	} {
		if err := call(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s(\"\") = %v, want a not-found error", name, err)
		}
	}
	got, err := x.Initiatives()
	if err != nil || len(got) != 1 || got[0].Slug != "" {
		t.Errorf("Initiatives = %+v, %v", got, err)
	}
}

// ---- Overview ----

func TestOverviewSortsEntriesByTitleIgnoringCaseThenID(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "banana", iniA),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Apple", iniA),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "cherry", iniA),
		"task/task_t4t4t4.md":        task("task_t4t4t4", "Banana", iniA),
	})
	o, err := open(t, w).Overview("alpha")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range o.Tasks {
		got = append(got, e.ID)
	}
	if want := "task_t2t2t2 task_t1t1t1 task_t4t4t4 task_t3t3t3"; strings.Join(got, " ") != want {
		t.Errorf("tasks = %v, want %s", got, want)
	}
}

func TestOverviewHoldsWhatTheREADMEShows(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": record(iniA, "Make it robust",
			"slug: alpha", "status: active", "purpose: it should cope",
			"goals:", "  - First goal", "  - Second goal",
			"involves:", "  - {to: who_k1m2n3, role: decides}", "  - {to: who_k4m5n6, role: consulted}"),
		"actor/who_k1m2n3.md":        record("who_k1m2n3", "Zed", "kind: person"),
		"actor/who_k4m5n6.md":        record("who_k4m5n6", "Ann", "kind: person"),
		"design-doc/dd_d1d1d1.md":    record("dd_d1d1d1", "The design", "status: accepted", "part_of: "+iniA),
		"prd/prd_p1p1p1.md":          record("prd_p1p1p1", "The PRD", "status: leaning", "part_of: "+iniA),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "Same title", iniA, "progress: done"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Same title", iniA, "progress: in_progress"),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Another", iniA),
		"task/task_t4t4t4.md":        task("task_t4t4t4", "Elsewhere", iniB),
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
	})
	valid(t, w)
	o, err := open(t, w).Overview("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if o.Initiative.ID != iniA || o.Initiative.Title != "Make it robust" || o.Slug != "alpha" || o.Purpose != "it should cope" {
		t.Errorf("header = %+v", o)
	}
	if o.Path != "graph/initiative/"+iniA+".md" || !strings.Contains(o.Body, "Body of "+iniA) {
		t.Errorf("path %q, body %q", o.Path, o.Body)
	}
	if !reflect.DeepEqual(o.Goals, []string{"First goal", "Second goal"}) {
		t.Errorf("goals = %v", o.Goals)
	}
	if len(o.People) != 2 || o.People[0].Title != "Zed" || o.People[0].Role != "decides" || o.People[1].Title != "Ann" || o.People[1].Role != "consulted" {
		t.Errorf("people = %+v, want the record's order, not sorted", o.People)
	}
	if len(o.DesignDocs) != 1 || o.DesignDocs[0].ID != "dd_d1d1d1" || o.DesignDocs[0].Path != "graph/design-doc/dd_d1d1d1.md" || o.DesignDocs[0].Status != "accepted" {
		t.Errorf("design docs = %+v", o.DesignDocs)
	}
	if len(o.PRDs) != 1 || o.PRDs[0].Status != "leaning" || o.PRDs[0].Progress != "" {
		t.Errorf("prds = %+v", o.PRDs)
	}
	var tasks []string
	for _, e := range o.Tasks {
		tasks = append(tasks, e.ID+":"+e.Progress)
	}
	want := []string{"task_t2t2t2:todo", "task_t1t1t1:in_progress", "task_t3t3t3:done"}
	if !reflect.DeepEqual(tasks, want) {
		t.Errorf("tasks = %v, want %v (by title, then ID; another initiative's task left out)", tasks, want)
	}
}

func TestOverviewListsFindingsMadeInTheInitiativesActivities(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
		"activity/actv_a1a1a1.md":    record("actv_a1a1a1", "Session one", "started_at: 2026-11-22", "part_of: "+iniA),
		"activity/actv_b2b2b2.md":    record("actv_b2b2b2", "Session two", "started_at: 2026-11-22", "part_of: "+iniB),
		"finding/fnd_f1f1f1.md":      record("fnd_f1f1f1", "banana", "generated_by: [actv_a1a1a1]"),
		"finding/fnd_f2f2f2.md":      record("fnd_f2f2f2", "Apple", "generated_by: [actv_a1a1a1]"),
		"finding/fnd_f3f3f3.md":      record("fnd_f3f3f3", "Elsewhere", "generated_by: [actv_b2b2b2]"),
		"finding/fnd_f4f4f4.md":      record("fnd_f4f4f4", "Loose"),
		"evidence/ev_e1e1e1.md":      record("ev_e1e1e1", "Seen", "observed_at: 2026-11-22", "from: [who_k1m2n3]", "generated_by: [actv_a1a1a1]"),
		"actor/who_k1m2n3.md":        record("who_k1m2n3", "Zed", "kind: person"),
	})
	x := open(t, w)
	o, err := x.Overview("alpha")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range o.Findings {
		got = append(got, e.ID+":"+e.Status+":"+e.Path)
	}
	want := []string{"fnd_f2f2f2::graph/finding/fnd_f2f2f2.md", "fnd_f1f1f1::graph/finding/fnd_f1f1f1.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v (by title, no status; evidence, another initiative's and an unattached finding left out)", got, want)
	}
	var rows int
	if err := x.db.QueryRow(`SELECT count(*) FROM produced WHERE initiative = ?`, iniA).Scan(&rows); err != nil || rows != 3 {
		t.Errorf("produced rows for %s = %d, %v; want 3 (two findings and the evidence)", iniA, rows, err)
	}
}

func TestOverviewOfAnUnknownSlug(t *testing.T) {
	w := newWS(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	_, err := open(t, w).Overview("alpa")
	var nf *NotFoundError
	if !errors.As(err, &nf) || len(nf.Close) == 0 || nf.Close[0] != "alpha" {
		t.Errorf("Overview(alpa) = %v", err)
	}
}

func TestOverviewWithNoGoalsHasAnEmptyList(t *testing.T) {
	w := newWS(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	o, err := open(t, w).Overview("alpha")
	if err != nil || o.Goals == nil || len(o.Goals) != 0 || len(o.People) != 0 {
		t.Errorf("Overview = %+v, %v", o, err)
	}
}

// ---- stable ----

func TestWhereAndOverviewAreEqualAcrossTwoBuilds(t *testing.T) {
	records := questionsWS()
	records["task/task_t1t1t1.md"] = task("task_t1t1t1", "Working", iniA, "progress: blocked", "blocked_by: [q_c1c1c1]")
	records["belief/bel_b1b1b1.md"] = belief("bel_b1b1b1", "held")
	records["task/task_t2t2t2.md"] = task("task_t2t2t2", "Relies", iniA, pinned("bel_b1b1b1"))
	w := newWS(t, records)

	read := func() (*View, *Overview) {
		x := open(t, w)
		o, err := x.Overview("alpha")
		if err != nil {
			t.Fatal(err)
		}
		return where(t, x, "alpha"), o
	}
	v1, o1 := read()
	removeAll(t, w.Root)
	v2, o2 := read()
	if !reflect.DeepEqual(v1, v2) || !reflect.DeepEqual(o1, o2) {
		t.Errorf("two builds differ:\n%+v\n%+v\n%+v\n%+v", v1, v2, o1, o2)
	}
	if len(v1.Open) == 0 || len(v1.Blocked) == 0 || len(v1.Recheck) == 0 {
		t.Errorf("the scenario should fill every section: %+v", v1)
	}
}

// valid fails the test when the workspace's records have errors, so a
// test can't pass by reading records the schema rejects.
func valid(t *testing.T, w *testws.Workspace) {
	t.Helper()
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	if errs, _ := res.Diagnostics.Count(); errs != 0 {
		t.Fatalf("the records have errors: %v", res.Diagnostics)
	}
}

// ---- Where: orphaned and doubted links, pending answers ----

// recheckOf lists the re-check entries of an initiative as
// "target <- source.field (flags)".
func recheckOf(v *View) []string {
	var got []string
	for _, c := range v.Recheck {
		for _, l := range c.Links {
			var flags []string
			for flag, on := range map[string]bool{"stale": l.Stale, "reopens": l.Reopens, "orphaned": l.Orphaned, "doubted": l.Doubted} {
				if on {
					flags = append(flags, flag)
				}
			}
			slices.Sort(flags)
			got = append(got, c.Target.ID+" <- "+l.Other.ID+"."+l.Field+" ("+strings.Join(flags, ",")+")")
		}
	}
	return got
}

func TestAcceptedTaskOnAWithdrawnBeliefNeedsARecheck(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"belief/bel_b1b1b1.md":       belief("bel_b1b1b1", "withdrawn"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Relies", iniA, "relies_on: [bel_b1b1b1]"),
		"task/task_t2t2t2.md":        record("task_t2t2t2", "Rejected", "status: rejected", "part_of: "+iniA, "relies_on: [bel_b1b1b1]"),
	})
	valid(t, w)
	x := open(t, w)
	got, want := recheckOf(where(t, x, "alpha")), []string{"bel_b1b1b1 <- task_t1t1t1.relies_on (orphaned)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recheck = %v, want %v (a rejected task is no longer in force)", got, want)
	}
	if r, err := x.Reopened(); err != nil || len(r) != 0 {
		t.Errorf("Reopened = %+v, %v; a withdrawn belief reopens nothing", r, err)
	}
	stale, err := x.Stale()
	if err != nil || len(stale) != 1 || len(stale[0].Links) != 1 || stale[0].Links[0].Other.ID != "task_t1t1t1" || !stale[0].Links[0].Orphaned {
		t.Errorf("Stale = %+v, %v", stale, err)
	}
}

func TestPlanDerivedFromASupersededTaskNeedsARecheck(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        record("task_t1t1t1", "Old", "status: superseded", "part_of: "+iniA),
		"plan/plan_p1p1p1.md":        record("plan_p1p1p1", "Live", "status: accepted", "part_of: "+iniA, "derived_from: [task_t1t1t1]"),
		"plan/plan_p2p2p2.md":        record("plan_p2p2p2", "Dead", "status: rejected", "part_of: "+iniA, "derived_from: [task_t1t1t1]"),
	})
	got, want := recheckOf(where(t, open(t, w), "alpha")), []string{"task_t1t1t1 <- plan_p1p1p1.derived_from (orphaned)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recheck = %v, want %v", got, want)
	}
}

// doubtWS has an accepted decision of the initiative relying on a held
// belief with one support link, and evidence that can doubt it.
func doubtWS(extra map[string]string) map[string]string {
	records := map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"belief/bel_b1b1b1.md": record("bel_b1b1b1", "Belief", "claim: it is so", "status: held", "asserted_by: agent",
			"support: [{id: s1, evidence: ev_e1e1e1, direction: supports}]"),
		"evidence/ev_e1e1e1.md":  record("ev_e1e1e1", "Old", "observed_at: 2026-11-22T10:00:00Z", "from: []"),
		"decision/dec_d1d1d1.md": record("dec_d1d1d1", "Dec", "status: accepted", "part_of: "+iniA, "relies_on: [bel_b1b1b1]"),
	}
	for k, v := range extra {
		records[k] = v
	}
	return records
}

func TestBeliefWithDoubtedSupportNeedsARecheck(t *testing.T) {
	want := []string{"bel_b1b1b1 <- dec_d1d1d1.relies_on (doubted)"}
	cases := map[string]map[string]string{
		"undermined evidence": {"evidence/ev_e2e2e2.md": record("ev_e2e2e2", "New", "observed_at: 2026-11-23T10:00:00Z", "from: []", "undermines: [ev_e1e1e1]")},
		"undercut link":       {"evidence/ev_e2e2e2.md": record("ev_e2e2e2", "New", "observed_at: 2026-11-23T10:00:00Z", "from: []", "undercuts: [bel_b1b1b1/s1]")},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWS(t, doubtWS(extra))
			if got := recheckOf(where(t, open(t, w), "alpha")); !reflect.DeepEqual(got, want) {
				t.Errorf("recheck = %v, want %v", got, want)
			}
			edit(t, w, "belief/bel_b1b1b1.md", "support: [{id: s1, evidence: ev_e1e1e1, direction: supports}]", "support: []")
			if got := recheckOf(where(t, open(t, w), "alpha")); len(got) != 0 {
				t.Errorf("after removing the link, recheck = %v", got)
			}
		})
	}
	t.Run("status change", func(t *testing.T) {
		w := newWS(t, doubtWS(map[string]string{"evidence/ev_e2e2e2.md": record("ev_e2e2e2", "New", "observed_at: 2026-11-23T10:00:00Z", "from: []", "undermines: [ev_e1e1e1]")}))
		edit(t, w, "belief/bel_b1b1b1.md", "status: held", "status: insufficient")
		for _, s := range recheckOf(where(t, open(t, w), "alpha")) {
			if strings.Contains(s, "doubted") {
				t.Errorf("a fallen belief is settled, not doubted: %v", s)
			}
		}
	})
}

func TestQuestionWithAHeldBeliefAndAPendingAnswerStaysOpen(t *testing.T) {
	records := questionsWS()
	records["decision/dec_d3d3d3.md"] = decision("dec_d3d3d3", iniA, "proposed", "q_c3c3c3")
	w := newWS(t, records)
	var got string
	for _, q := range where(t, open(t, w), "alpha").Open {
		if q.ID == "q_c3c3c3" {
			got = ids(q.Pending)
		}
	}
	if got != "dec_d3d3d3" {
		t.Errorf("q3's pending answers = %q, want dec_d3d3d3 (the held belief settles it, but the decision still waits)", got)
	}
}
