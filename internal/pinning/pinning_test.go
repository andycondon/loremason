package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/testws"
)

func load(t *testing.T, records map[string]string) *graph.Graph {
	t.Helper()
	return testws.New(t, records).Load()
}

// A requirement with three revisions; revision 2 is breaking or not.
func requirement(r2 string) string {
	return `---
id: req_5txk3a
title: Unreadable files
statement: validate reports unreadable files clearly
status: accepted
revisions:
  - {rev: 1, date: 2026-11-22, fp: "fp1:000000000001", statement: a}
  - {rev: 2, date: 2026-11-23, fp: "fp1:000000000002", change: ` + r2 + `, statement: b}
  - {rev: 3, date: 2026-11-24, fp: "fp1:000000000003", change: non_breaking}
created: 2026-11-22
---
`
}

const task = `---
id: task_9w4hzd
title: Report them
status: accepted
implements:
  - {to: req_5txk3a, rev: 1, fp: "fp1:000000000001"}
relies_on: [req_5txk3a@r1]
created: 2026-11-22
---
`

func actions(p *Plan) string {
	var out []string
	for _, a := range p.Actions {
		out = append(out, a.String())
	}
	return strings.Join(out, "\n")
}

func TestRepinOnlyAcrossNonBreakingRevisions(t *testing.T) {
	quiet := actions(Sync(load(t, map[string]string{"requirement/req_5txk3a.md": requirement("non_breaking"), "task/task_9w4hzd.md": task}), nil, today))
	if !strings.Contains(quiet, "re-pin `implements[0]` to req_5txk3a@r3") {
		t.Errorf("revisions 2 and 3 are non-breaking, so the pin should move:\n%s", quiet)
	}
	held := actions(Sync(load(t, map[string]string{"requirement/req_5txk3a.md": requirement("breaking"), "task/task_9w4hzd.md": task}), nil, today))
	if strings.Contains(held, "re-pin") {
		t.Errorf("revision 2 is breaking, so the link needs a re-check, not a quiet re-pin:\n%s", held)
	}
}

func TestBareLinkPinsTheRevisionItNames(t *testing.T) {
	p := Sync(load(t, map[string]string{"requirement/req_5txk3a.md": requirement("breaking"), "task/task_9w4hzd.md": task}), nil, today)
	// Revision 2 is breaking, so the link stays on the revision it names.
	if got := actions(p); !strings.Contains(got, "pin `relies_on[0]` to req_5txk3a@r1") || strings.Contains(got, "since every revision") {
		t.Errorf("got:\n%s", actions(p))
	}
}

func TestBrokenFilesAndLinksToThemAreSkipped(t *testing.T) {
	g := load(t, map[string]string{"requirement/req_5txk3a.md": requirement("non_breaking"), "task/task_9w4hzd.md": task})
	p := Sync(g, map[string]bool{"graph/requirement/req_5txk3a.md": true}, today)
	// The task itself is fine, so it still gets its revision 1; only its
	// links to the broken requirement are left alone.
	if actions(p) != "graph/task/task_9w4hzd.md: write revision 1" || len(p.Skipped) != 1 {
		t.Errorf("actions:\n%s\nskipped: %v", actions(p), p.Skipped)
	}
}

func TestApplyRefusesAFileThatChangedAndWritesNothing(t *testing.T) {
	files := map[string]string{"requirement/req_5txk3a.md": requirement("non_breaking"), "task/task_9w4hzd.md": task}
	w := testws.New(t, files)
	p := Sync(w.Load(), nil, today) // writes to both files
	w.Write("task/task_9w4hzd.md", task+"\nEdited meanwhile.\n")
	if err := Apply(w.Root, p); err == nil || !strings.Contains(err.Error(), "changed after it was read") {
		t.Fatalf("got %v", err)
	}
	if w.Read("requirement/req_5txk3a.md") != files["requirement/req_5txk3a.md"] {
		t.Error("another file was written before the refusal")
	}
}
