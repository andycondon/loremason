package pinning

import (
	"slices"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/testws"
)

// A belief changes, and only what's built on it is re-checked. This is the
// worked example in docs/context-graph/03-implementation.md ("Worked
// example: a belief changes"), one test per outcome.

const beliefChangePlan = `---
id: plan_k2x8wd
title: Plan for reporting unreadable files
status: accepted
derived_from: task_9w4hzd
created: 2026-11-22
---

Catch the error where it happens, then report it, then test it.
`

func beliefChangeStep(id, title string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\npart_of: plan_k2x8wd\ncreated: 2026-11-22\n---\n\nChange the read call; check with one test.\n"
}

// beliefChangeBefore is step 1: a task relies on the belief, a plan is derived
// from the task, and its three steps are part of the plan, all pinned.
func beliefChangeBefore(t *testing.T) *testws.Workspace {
	t.Helper()
	w := testws.New(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"task/task_9w4hzd.md":  testws.Task,
		"plan/plan_k2x8wd.md":  beliefChangePlan,
		"step/step_2c7rq1.md":  beliefChangeStep("step_2c7rq1", "Catch the decode error"),
		"step/step_8v0ynf.md":  beliefChangeStep("step_8v0ynf", "Report the file"),
		"step/step_d3m6hk.md":  beliefChangeStep("step_d3m6hk", "Add a fixture per encoding"),
	})
	sync(t, w)
	if stale := w.Stale(); len(stale) != 0 {
		t.Fatalf("step 1: stale links before anything changed: %v", stale)
	}
	return w
}

// beliefChangeRevised is steps 2 and 3: the claim narrows to Latin-1, as a
// breaking revision, and only the task's link goes stale.
func beliefChangeRevised(t *testing.T) *testws.Workspace {
	t.Helper()
	w := beliefChangeBefore(t)
	revise(t, w, "bel_7k3m9q", true)
	w.Edit("belief/bel_7k3m9q.md", "claim: validate doesn't handle non-UTF-8 input", "claim: validate doesn't handle Latin-1 input")
	sync(t, w)
	if bel := w.Node("bel_7k3m9q"); bel.Current().Rev != 2 || !bel.Current().Breaking || bel.Revision(1).KeptText("claim") != "validate doesn't handle non-UTF-8 input" {
		t.Fatalf("step 2: the revision isn't as described: %+v", bel.Revisions)
	}
	if stale := w.Stale(); !slices.Equal(stale, []string{"task_9w4hzd relies_on -> bel_7k3m9q"}) {
		t.Fatalf("step 3: only the task should be stale, got %v", stale)
	}
	return w
}

func TestBeliefChangeStopsAtTheTaskThatStillHolds(t *testing.T) {
	w := beliefChangeRevised(t)
	planBefore := w.Read("plan/plan_k2x8wd.md")

	// Step 4: an agent re-reads the task, finds it still holds, writes
	// evidence saying why, and moves the pin with via pointing at it.
	w.Write("evidence/ev_m3q8vb.md", "---\nid: ev_m3q8vb\ntitle: The task still holds\nobserved_at: 2026-11-29\nabout: task_9w4hzd\ncreated: 2026-11-29\n---\n\nStill holds: the outcome doesn't depend on which encodings fail.\n")
	g := w.Load()
	var link *graph.Edge
	for _, e := range g.Edges {
		if e.Stale() {
			link = e
		}
	}
	plan, err := Recheck(link, g.Node("ev_m3q8vb"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(w.Root, plan); err != nil {
		t.Fatal(err)
	}

	if stale := w.Stale(); len(stale) != 0 {
		t.Errorf("step 4: still stale: %v", stale)
	}
	task := w.Read("task/task_9w4hzd.md")
	if !strings.Contains(task, "{to: bel_7k3m9q, rev: 2, fp: \""+w.Node("bel_7k3m9q").Fingerprint().String()+"\", via: ev_m3q8vb}") {
		t.Errorf("step 4: the pin doesn't record the re-check:\n%s", task)
	}
	if n := w.Node("task_9w4hzd"); len(n.Revisions) != 1 {
		t.Errorf("step 4: the task got a new revision: %+v", n.Revisions)
	}
	if w.Read("plan/plan_k2x8wd.md") != planBefore {
		t.Error("step 4: the plan was touched")
	}
	if errs, _ := checked(t, w).Diagnostics.Count(); errs != 0 {
		t.Error(problems(t, w))
	}
	if p := sync(t, w); !p.Empty() {
		t.Errorf("step 4: sync after the re-check: %s", actions(p))
	}
}

func TestBeliefChangeReachesThePlanWhenTheTaskChanges(t *testing.T) {
	w := beliefChangeRevised(t)

	// Step 5: the task's approach named UTF-16 detection, so the task gets a
	// breaking revision. Now the plan's link is stale, and nothing below it.
	revise(t, w, "task_9w4hzd", true)
	w.Edit("task/task_9w4hzd.md", "validate names any file it can't read.", "validate names any file it can't read, and the encoding it expected.")
	sync(t, w)
	stale := w.Stale()
	if !slices.Contains(stale, "plan_k2x8wd derived_from -> task_9w4hzd") || slices.ContainsFunc(stale, func(s string) bool { return strings.HasPrefix(s, "step_") }) {
		t.Errorf("step 5: the plan, and not its steps, should be stale: %v", stale)
	}
}

func TestCosmeticEditChangesNothing(t *testing.T) {
	w := beliefChangeBefore(t)

	// Step 6: only the belief's reasoning is re-wrapped. Its meaning fields
	// don't change, so neither does its fingerprint.
	w.Edit("belief/bel_7k3m9q.md", "Both failures are on files that aren't UTF-8.", "Both failures are on files\nthat aren't UTF-8.")
	if p := sync(t, w); !p.Empty() {
		t.Errorf("step 6: %s", actions(p))
	}
	if stale := w.Stale(); len(stale) != 0 {
		t.Errorf("step 6: stale: %v", stale)
	}
	if n := w.Node("bel_7k3m9q"); len(n.Revisions) != 1 {
		t.Errorf("step 6: a revision was made: %+v", n.Revisions)
	}
}
