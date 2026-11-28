package index

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/testws"
)

// ---- the index is a cache ----

// answers is everything the queries say about a workspace.
type answers struct {
	Stale      []Change
	Reopened   []Reopening
	Dependents []Dependent
	Show       *Detail
	Found      []Summary
}

func ask(t *testing.T, x *Index, id string) answers {
	t.Helper()
	a, err := answersOf(x, id)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// answersOf runs every query, for tests that can't call t.Fatal.
func answersOf(x *Index, id string) (a answers, err error) {
	if a.Stale, err = x.Stale(); err != nil {
		return
	}
	if a.Reopened, err = x.Reopened(); err != nil {
		return
	}
	if a.Dependents, err = x.Dependents(id); err != nil {
		return
	}
	if a.Show, err = x.Show(id); err != nil {
		return
	}
	a.Found, err = x.Find("utf 8")
	return
}

func TestDeletingTheIndexChangesNoAnswer(t *testing.T) {
	w := workedExample(t)
	edit(t, w, "requirement/req_5txk3a.md", "created:", `revisions:
  - {rev: 1, date: 2026-11-22, fp: "fp1:000000000001", statement: a}
  - {rev: 2, date: 2026-11-23, fp: "fp1:000000000002", change: breaking, statement: b}
created:`)
	edit(t, w, "task/task_9w4hzd.md", "implements: [req_5txk3a]", `implements: [{to: req_5txk3a, rev: 1, fp: "fp1:000000000001"}]`)
	edit(t, w, "belief/bel_7k3m9q.md", "status: held", "status: incorrect")

	x := open(t, w)
	before := ask(t, x, "bel_7k3m9q")
	if len(before.Stale) != 2 || len(before.Reopened) < 2 || len(before.Dependents) == 0 || len(before.Show.In) == 0 || len(before.Found) == 0 {
		t.Fatalf("the scenario should have a stale pin, a fallen belief and dependents: %+v", before)
	}
	x.Close()

	if err := removeIndex(w); err != nil {
		t.Fatal(err)
	}
	x = open(t, w)
	if !x.Rebuilt() {
		t.Fatal("deleting the index should make the next open build it")
	}
	if after := ask(t, x, "bel_7k3m9q"); !reflect.DeepEqual(before, after) {
		t.Errorf("answers changed:\nbefore %+v\nafter  %+v", before, after)
	}
}

// ---- Find ----

func TestFindByPlainWords(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": strings.Replace(testws.Belief, "Both failures are on files that aren't UTF-8.", "Failures are on non-UTF-8 files.", 1)})
	x := open(t, w)
	for _, words := range []string{"utf-8 files", "UTF-8", "non-utf-8 FILES", "validate"} {
		got, err := x.Find(words)
		if err != nil {
			t.Fatalf("%q: %v", words, err)
		}
		want := []Summary{{ID: "bel_7k3m9q", Type: "belief", Title: "validate and encodings", Status: "held"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Find(%q) = %+v, want %+v", words, got, want)
		}
	}
}

func TestFindSurvivesSyntaxInTheWords(t *testing.T) {
	x := open(t, newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief}))
	for _, words := range []string{`"`, `utf"`, `files AND`, `NEAR(`, `a OR`, `*`, `'; DROP TABLE nodes; --`, `title:x`, `-`, `(`} {
		if _, err := x.Find(words); err != nil {
			t.Errorf("Find(%q): %v", words, err)
		}
	}
	for _, words := range []string{"", "  ", "-- !!"} {
		if got, err := x.Find(words); err != nil || got != nil {
			t.Errorf("Find(%q) = %v, %v; want nothing", words, got, err)
		}
	}
	if got, _ := x.Find("nothingmatchesthis"); len(got) != 0 {
		t.Errorf("found %v", got)
	}
}

func TestFindOrdersByID(t *testing.T) {
	x := open(t, newWS(t, map[string]string{
		"belief/bel_c9d4se.md": strings.Replace(testws.Belief, "bel_7k3m9q", "bel_c9d4se", 1),
		"belief/bel_7k3m9q.md": testws.Belief,
	}))
	got, err := x.Find("validate")
	if err != nil || ids(got) != "bel_7k3m9q bel_c9d4se" {
		t.Errorf("Find = %s, %v", ids(got), err)
	}
}

// ---- Show ----

func TestShowNamesLinksByTitle(t *testing.T) {
	x := open(t, workedExample(t))
	d, err := x.Show("task_9w4hzd")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "task" || d.Title != "Report unreadable files clearly" || d.Status != "accepted" {
		t.Errorf("summary = %+v", d.Summary)
	}
	if !strings.Contains(d.Body, "validate names any file") || !strings.Contains(d.Header, `"relies_on":["bel_7k3m9q"]`) {
		t.Errorf("body or header missing: %q %q", d.Body, d.Header)
	}
	out := map[string]Link{}
	for _, l := range d.Out {
		out[l.Field+" "+l.Other.ID] = l
	}
	rel, ok := out["relies_on bel_7k3m9q"]
	if !ok || rel.Other.Title != "validate doesn't handle non-UTF-8 input" || rel.Other.Type != "belief" {
		t.Errorf("relies_on link = %+v (all: %+v)", rel, d.Out)
	}
	if part, ok := out["part_of ini_2m8vqe"]; !ok || part.Other.Title == "" {
		t.Errorf("part_of link = %+v", part)
	}

	b, err := x.Show("bel_7k3m9q")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range b.In {
		if l.Other.ID == "task_9w4hzd" && l.Field == "relies_on" && l.Other.Title == "Report unreadable files clearly" {
			found = true
		}
	}
	if !found {
		t.Errorf("In of the belief lacks the task: %+v", b.In)
	}
}

func TestShowReportsPins(t *testing.T) {
	x := open(t, newWS(t, map[string]string{
		"requirement/req_5txk3a.md": requirement("req_5txk3a", "non_breaking"),
		"task/task_9w4hzd.md":       record("task_9w4hzd", "Task", "status: accepted", `implements: [{to: req_5txk3a, rev: 1, fp: "fp1:000000000001"}]`, "relies_on: []"),
	}))
	d, err := x.Show("task_9w4hzd")
	if err != nil {
		t.Fatal(err)
	}
	want := []Link{{Field: "implements", Pin: &Pin{Rev: 1, FP: "fp1:000000000001"}, Other: Summary{ID: "req_5txk3a", Type: "requirement", Title: "Unreadable files", Status: "accepted"}}}
	if !reflect.DeepEqual(d.Out, want) {
		t.Errorf("Out = %+v, want %+v", d.Out, want)
	}
}

func TestShowResolvesAliases(t *testing.T) {
	x := open(t, newWS(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(testws.Belief, "created:", "aliases: [encodings, enc/utf]\ncreated:", 1),
		"question/q_3fz8km.md": `---
id: q_3fz8km
question: Which?
aliases: [which]
options:
  - {id: o1, title: One, for: [encodings]}
created: 2026-11-22
---
`,
		"task/task_9w4hzd.md": record("task_9w4hzd", "Task", "status: accepted", "relies_on: [encodings]"),
	}))
	for name, want := range map[string]string{
		"bel_7k3m9q":  "bel_7k3m9q",
		"encodings":   "bel_7k3m9q",
		"enc/utf":     "bel_7k3m9q",
		"q_3fz8km/o1": "q_3fz8km/o1",
		"which/o1":    "q_3fz8km/o1",
	} {
		d, err := x.Show(name)
		if err != nil || d.ID != want {
			t.Errorf("Show(%s) = %+v, %v; want %s", name, d, err, want)
		}
	}
	got, err := x.Dependents("encodings")
	if err != nil || ids2(got) != "task_9w4hzd" {
		t.Errorf("Dependents(alias) = %+v, %v", got, err)
	}
	d, _ := x.Show("task_9w4hzd")
	if len(d.Out) != 1 || d.Out[0].Other.ID != "bel_7k3m9q" {
		t.Errorf("a link through an alias should end at the record: %+v", d.Out)
	}
}

func TestShowUnknownIDSaysWhatIsClose(t *testing.T) {
	x := open(t, workedExample(t))
	_, err := x.Show("bel_7k3m9r")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	var nf *NotFoundError
	if !errors.As(err, &nf) || nf.ID != "bel_7k3m9r" || len(nf.Close) == 0 || nf.Close[0] != "bel_7k3m9q" {
		t.Errorf("NotFoundError = %+v", nf)
	}
	if !strings.Contains(err.Error(), "did you mean `bel_7k3m9q") {
		t.Errorf("message = %q", err)
	}
	if _, err := x.Dependents("zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Dependents of an unknown ID: %v", err)
	}
	_, err = x.Show("zzzzzzzzzzzzzzzzzzzz")
	if nf = nil; !errors.As(err, &nf) || len(nf.Close) != 0 || strings.Contains(err.Error(), "did you mean") {
		t.Errorf("nothing is close, but: %v", err)
	}
	_, err = x.Show("q_3fz8km/nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown local name: %v", err)
	}
}

// ---- Dependents ----

func TestDependentsGoFurtherDown(t *testing.T) {
	// A belief, a requirement relying on it, and a task implementing the
	// requirement. A finding that only mentions the belief isn't a dependent.
	x := open(t, newWS(t, map[string]string{
		"belief/bel_7k3m9q.md":      testws.Belief,
		"requirement/req_5txk3a.md": record("req_5txk3a", "Req", "status: accepted", "relies_on: [bel_7k3m9q]"),
		"task/task_9w4hzd.md":       record("task_9w4hzd", "Task", "status: accepted", "implements: [req_5txk3a]"),
		"finding/fnd_6r4czy.md":     record("fnd_6r4czy", "Finding", "mentions: [bel_7k3m9q]"),
	}))
	got, err := x.Dependents("bel_7k3m9q")
	if err != nil {
		t.Fatal(err)
	}
	want := []Dependent{
		{Summary: Summary{ID: "req_5txk3a", Type: "requirement", Title: "Req", Status: "accepted"}, Depth: 1, Field: "relies_on",
			On: Summary{ID: "bel_7k3m9q", Type: "belief", Title: "validate and encodings", Status: "held"}},
		{Summary: Summary{ID: "task_9w4hzd", Type: "task", Title: "Task", Status: "accepted"}, Depth: 2, Field: "implements",
			On: Summary{ID: "req_5txk3a", Type: "requirement", Title: "Req", Status: "accepted"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Dependents = %+v\nwant %+v", got, want)
	}
	if got, _ := x.Dependents("task_9w4hzd"); len(got) != 0 {
		t.Errorf("nothing relies on the task: %+v", got)
	}
}

func TestDependentsListEachOnceAtItsShortestDepth(t *testing.T) {
	// The task relies on the belief directly, and also reaches it through
	// the requirement: depth 1 wins. Containment doesn't propagate.
	x := open(t, newWS(t, map[string]string{
		"belief/bel_7k3m9q.md":      testws.Belief,
		"requirement/req_5txk3a.md": record("req_5txk3a", "Req", "status: accepted", "relies_on: [bel_7k3m9q]"),
		"task/task_9w4hzd.md":       record("task_9w4hzd", "Task", "status: accepted", "implements: [req_5txk3a]", "relies_on: [bel_7k3m9q]"),
		"question/q_3fz8km.md": `---
id: q_3fz8km
question: Which?
options:
  - {id: o1, title: One, for: [bel_7k3m9q]}
created: 2026-11-22
---
`,
	}))
	got, err := x.Dependents("bel_7k3m9q")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "req_5txk3a" || got[1].ID != "task_9w4hzd" || got[1].Depth != 1 {
		t.Errorf("Dependents = %+v", got)
	}
}

func TestDependentsStopAtACycle(t *testing.T) {
	// Two tasks implement each other. The index doesn't check types, so the
	// walk has to cope: the other task once, and not the start itself.
	x := open(t, newWS(t, map[string]string{
		"task/task_9w4hzd.md": record("task_9w4hzd", "A", "status: accepted", "implements: [task_c9d4se]"),
		"task/task_c9d4se.md": record("task_c9d4se", "B", "status: accepted", "implements: [task_9w4hzd]"),
	}))
	got, err := x.Dependents("task_9w4hzd")
	if err != nil || len(got) != 1 || got[0].ID != "task_c9d4se" || got[0].Depth != 1 {
		t.Errorf("Dependents = %+v, %v", got, err)
	}
}

func TestDependentsOfADenseGraphAreFast(t *testing.T) {
	// Every task implements every other: the number of paths is astronomical,
	// the number of records is not.
	const n = 60
	name := func(i int) string { return fmt.Sprintf("task_%06d", i) }
	files := map[string]string{}
	for i := 0; i < n; i++ {
		var links []string
		for j := 0; j < n; j++ {
			if j != i {
				links = append(links, name(j))
			}
		}
		files["task/"+name(i)+".md"] = record(name(i), name(i), "status: accepted", "implements: ["+strings.Join(links, ", ")+"]")
	}
	x := open(t, newWS(t, files))
	start := time.Now()
	got, err := x.Dependents(name(0))
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Dependents took %v", took)
	}
	if len(got) != n-1 {
		t.Fatalf("got %d dependents, want %d", len(got), n-1)
	}
	for i, d := range got {
		if d.Depth != 1 || d.On.ID != name(0) || d.ID != name(i+1) {
			t.Fatalf("dependent %d = %+v", i, d)
		}
	}
}

func TestShowListsSubRecordLinksUnderTheRecord(t *testing.T) {
	x := open(t, newWS(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"question/q_3fz8km.md": `---
id: q_3fz8km
question: Which?
options:
  - {id: o1, title: One, for: [bel_7k3m9q]}
created: 2026-11-22
---
`,
	}))
	d, err := x.Show("q_3fz8km")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, l := range d.Out {
		if l.Field == Containment {
			t.Errorf("containment row in Out: %+v", l)
		}
		if l.Other.ID == "bel_7k3m9q" && l.Local == "o1" {
			found = true
		}
	}
	if !found {
		t.Errorf("Out lacks the option's link marked o1: %+v", d.Out)
	}
	b, err := x.Show("bel_7k3m9q")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range b.In {
		if l.Field == Containment {
			t.Errorf("containment row in In: %+v", l)
		}
	}
}

func ids2(ds []Dependent) string {
	var s []Summary
	for _, d := range ds {
		s = append(s, d.Summary)
	}
	return ids(s)
}

// ---- Reopened ----

func TestFallenBeliefReopensCommitments(t *testing.T) {
	x := open(t, newWS(t, map[string]string{
		"belief/bel_7k3m9q.md":   strings.Replace(testws.Belief, "status: held", "status: incorrect", 1),
		"belief/bel_c9d4se.md":   strings.NewReplacer("bel_7k3m9q", "bel_c9d4se", "status: held", "status: withdrawn").Replace(testws.Belief),
		"decision/dec_6n1xtb.md": record("dec_6n1xtb", "Dec", "status: accepted", "relies_on: [bel_7k3m9q, bel_c9d4se]"),
		"task/task_9w4hzd.md":    record("task_9w4hzd", "Task", "status: accepted", "relies_on: [bel_7k3m9q]"),
	}))
	got, err := x.Reopened()
	if err != nil {
		t.Fatal(err)
	}
	belief := Summary{ID: "bel_7k3m9q", Type: "belief", Title: "validate and encodings", Status: "incorrect"}
	want := []Reopening{
		{Commitment: Summary{ID: "dec_6n1xtb", Type: "decision", Title: "Dec", Status: "accepted"}, Belief: belief, Field: "relies_on", Status: "incorrect"},
		{Commitment: Summary{ID: "task_9w4hzd", Type: "task", Title: "Task", Status: "accepted"}, Belief: belief, Field: "relies_on", Status: "incorrect"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Reopened = %+v\nwant %+v", got, want)
	}
}

func TestNothingReopenedWhenBeliefsHold(t *testing.T) {
	x := open(t, workedExample(t))
	if got, err := x.Reopened(); err != nil || len(got) != 0 {
		t.Errorf("Reopened = %+v, %v", got, err)
	}
}

// ---- Stale ----

func TestStaleIsGroupedByTheChangedRecord(t *testing.T) {
	pin := `implements: [{to: req_5txk3a, rev: 1, fp: "fp1:000000000001"}]`
	w := newWS(t, map[string]string{
		"requirement/req_5txk3a.md": requirement("req_5txk3a", "breaking"),
		"task/task_9w4hzd.md":       record("task_9w4hzd", "First", "status: accepted", pin),
		"task/task_b2c3d4.md":       record("task_b2c3d4", "Second", "status: accepted", pin),
	})
	// A task pinned to the current revision isn't stale.
	current := fmt.Sprintf(`implements: [{to: req_5txk3a, rev: 2, fp: %q}]`, w.Node("req_5txk3a").Fingerprint().String())
	put(t, w, "task/task_c3d4e5.md", record("task_c3d4e5", "Current", "status: accepted", current))
	x := open(t, w)
	got, err := x.Stale()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Stale = %+v, want one group", got)
	}
	g := got[0]
	if g.Target.ID != "req_5txk3a" || g.Revision != 2 || len(g.Links) != 2 {
		t.Fatalf("group = %+v", g)
	}
	for i, id := range []string{"task_9w4hzd", "task_b2c3d4"} {
		l := g.Links[i]
		if l.Other.ID != id || l.Field != "implements" || l.Pin == nil || l.Pin.Rev != 1 || l.Pin.FP != "fp1:000000000001" {
			t.Errorf("link %d = %+v", i, l)
		}
	}
}

func TestStaleOfACleanWorkspace(t *testing.T) {
	x := open(t, workedExample(t))
	// The worked example has no stale pin; what it lists is the doubted belief.
	got, err := x.Stale()
	if err != nil || len(got) != 1 || got[0].Target.ID != "bel_7k3m9q" {
		t.Fatalf("Stale = %+v, %v", got, err)
	}
	for _, l := range got[0].Links {
		if l.Concerns != graph.Doubted {
			t.Errorf("link %+v: concerns = %v, want only a doubt", l, l.Concerns)
		}
	}
}

// ---- errors ----

func TestQueriesOnAClosedIndex(t *testing.T) {
	x := open(t, workedExample(t))
	x.Close()
	if _, err := x.Find("utf"); err == nil {
		t.Error("Find on a closed index succeeded")
	}
	if _, err := x.Show("bel_7k3m9q"); err == nil {
		t.Error("Show on a closed index succeeded")
	}
	if _, err := x.Dependents("bel_7k3m9q"); err == nil {
		t.Error("Dependents on a closed index succeeded")
	}
	if _, err := x.Stale(); err == nil {
		t.Error("Stale on a closed index succeeded")
	}
	if _, err := x.Reopened(); err == nil {
		t.Error("Reopened on a closed index succeeded")
	}
}
