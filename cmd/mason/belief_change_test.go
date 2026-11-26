package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/pinning"
	"github.com/andycondon/loremason/internal/registry"
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
func beliefChangeBefore(t *testing.T) string {
	t.Helper()
	root := newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": beliefRecord,
		"task/task_9w4hzd.md":  taskRecord,
		"plan/plan_k2x8wd.md":  beliefChangePlan,
		"step/step_2c7rq1.md":  beliefChangeStep("step_2c7rq1", "Catch the decode error"),
		"step/step_8v0ynf.md":  beliefChangeStep("step_8v0ynf", "Report the file"),
		"step/step_d3m6hk.md":  beliefChangeStep("step_d3m6hk", "Add a fixture per encoding"),
	})
	must(t, "sync")
	if stale := staleLinks(t, root); len(stale) != 0 {
		t.Fatalf("step 1: stale links before anything changed: %v", stale)
	}
	return root
}

// beliefChangeRevised is steps 2 and 3: the claim narrows to Latin-1, as a
// breaking revision, and only the task's link goes stale.
func beliefChangeRevised(t *testing.T) string {
	t.Helper()
	root := beliefChangeBefore(t)
	must(t, "revise", "bel_7k3m9q", "--breaking")
	edit(t, root, "belief/bel_7k3m9q.md", "claim: validate doesn't handle non-UTF-8 input", "claim: validate doesn't handle Latin-1 input")
	must(t, "sync")
	if bel := graphNode(t, root, "bel_7k3m9q"); bel.Current().Rev != 2 || !bel.Current().Breaking || bel.Revision(1).KeptText("claim") != "validate doesn't handle non-UTF-8 input" {
		t.Fatalf("step 2: the revision isn't as described: %+v", bel.Revisions)
	}
	if stale := staleLinks(t, root); !slices.Equal(stale, []string{"task_9w4hzd relies_on -> bel_7k3m9q"}) {
		t.Fatalf("step 3: only the task should be stale, got %v", stale)
	}
	return root
}

func TestBeliefChangeStopsAtTheTaskThatStillHolds(t *testing.T) {
	root := beliefChangeRevised(t)
	planBefore := read(t, root, "plan/plan_k2x8wd.md")

	// Step 4: an agent re-reads the task, finds it still holds, writes
	// evidence saying why, and moves the pin with via pointing at it.
	write(t, root, "evidence/ev_m3q8vb.md", "---\nid: ev_m3q8vb\ntitle: The task still holds\nobserved_at: 2026-11-29\nabout: task_9w4hzd\ncreated: 2026-11-29\n---\n\nStill holds: the outcome doesn't depend on which encodings fail.\n")
	g, _, err := graph.Load(root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	var link *graph.Edge
	for _, e := range g.Edges {
		if e.Stale() {
			link = e
		}
	}
	plan, err := pinning.Recheck(link, g.Node("ev_m3q8vb"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pinning.Apply(root, plan); err != nil {
		t.Fatal(err)
	}

	if stale := staleLinks(t, root); len(stale) != 0 {
		t.Errorf("step 4: still stale: %v", stale)
	}
	task := read(t, root, "task/task_9w4hzd.md")
	if !strings.Contains(task, "{to: bel_7k3m9q, rev: 2, fp: \""+graphNode(t, root, "bel_7k3m9q").Fingerprint().String()+"\", via: ev_m3q8vb}") {
		t.Errorf("step 4: the pin doesn't record the re-check:\n%s", task)
	}
	if n := graphNode(t, root, "task_9w4hzd"); len(n.Revisions) != 1 {
		t.Errorf("step 4: the task got a new revision: %+v", n.Revisions)
	}
	if read(t, root, "plan/plan_k2x8wd.md") != planBefore {
		t.Error("step 4: the plan was touched")
	}
	if out := must(t, "validate"); !strings.Contains(out, " 0 errors") {
		t.Error(out)
	}
	if out := must(t, "sync"); out != "nothing to sync\n" {
		t.Errorf("step 4: sync after the re-check: %s", out)
	}
}

func TestBeliefChangeReachesThePlanWhenTheTaskChanges(t *testing.T) {
	root := beliefChangeRevised(t)

	// Step 5: the task's approach named UTF-16 detection, so the task gets a
	// breaking revision. Now the plan's link is stale, and nothing below it.
	must(t, "revise", "task_9w4hzd", "--breaking")
	edit(t, root, "task/task_9w4hzd.md", "validate names any file it can't read.", "validate names any file it can't read, and the encoding it expected.")
	must(t, "sync")
	stale := staleLinks(t, root)
	if !slices.Contains(stale, "plan_k2x8wd derived_from -> task_9w4hzd") || slices.ContainsFunc(stale, func(s string) bool { return strings.HasPrefix(s, "step_") }) {
		t.Errorf("step 5: the plan, and not its steps, should be stale: %v", stale)
	}
}

func TestCosmeticEditChangesNothing(t *testing.T) {
	root := beliefChangeBefore(t)

	// Step 6: only the belief's reasoning is re-wrapped. Its meaning fields
	// don't change, so neither does its fingerprint.
	edit(t, root, "belief/bel_7k3m9q.md", "Both failures are on files that aren't UTF-8.", "Both failures are on files\nthat aren't UTF-8.")
	if out := must(t, "sync"); out != "nothing to sync\n" {
		t.Errorf("step 6: %s", out)
	}
	if stale := staleLinks(t, root); len(stale) != 0 {
		t.Errorf("step 6: stale: %v", stale)
	}
	if n := graphNode(t, root, "bel_7k3m9q"); len(n.Revisions) != 1 {
		t.Errorf("step 6: a revision was made: %+v", n.Revisions)
	}
}
