package pinning

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// Sync, revise and edit in the order people use them, on files written to
// disk.

func TestSyncPinsLinksAndTouchesNothingElse(t *testing.T) {
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": testws.Task})
	sync(t, w)
	got := w.Read("task/task_9w4hzd.md")
	pinned := "relies_on: # the one belief this rests on\n  - {to: bel_7k3m9q, rev: 1, fp: \"" + testws.BeliefFP + "\"}\n"
	if !strings.Contains(got, pinned) {
		t.Fatalf("the link isn't pinned as expected:\n%s", got)
	}
	// Everything but relies_on and the new revisions is byte-for-byte the same.
	rest := got[:strings.Index(got, "relies_on:")] + got[strings.Index(got, "created:"):]
	want := testws.Task[:strings.Index(testws.Task, "relies_on:")] + testws.Task[strings.Index(testws.Task, "created:"):]
	if rest != want {
		t.Errorf("lines nobody touched changed:\n%s", got)
	}
	if p := sync(t, w); !p.Empty() {
		t.Errorf("a second sync did something:\n%s", actions(p))
	}
	if got := problems(t, w); got != "" {
		t.Error(got)
	}
}

func TestDraftsStayCheap(t *testing.T) {
	w := testws.New(t, map[string]string{"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: Reading files safely\nstatus: proposed\ncreated: 2026-11-22\n---\n\nFirst draft.\n"})
	sync(t, w)
	w.Edit("design-doc/dd_c5r1tw.md", "First draft.", "Second draft, much better.")
	if got := actions(sync(t, w)); !strings.Contains(got, "refresh the fingerprint of revision 1, which no propagating link pins") {
		t.Errorf("got %s", got)
	}
	if n := w.Node("dd_c5r1tw"); len(n.Revisions) != 1 || n.Current().FP != n.Fingerprint() {
		t.Errorf("revisions: %+v", n.Revisions)
	}
}

func TestReviseThenEdit(t *testing.T) {
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": testws.Task})
	sync(t, w)

	// Editing a pinned claim without revising is caught.
	w.Edit("belief/bel_7k3m9q.md", "non-UTF-8 input", "Latin-1 input")
	if got := problems(t, w); !strings.Contains(got, "run `mason revise bel_7k3m9q --breaking` (or `--non-breaking`)") {
		t.Errorf("got %s", got)
	}
	w.Edit("belief/bel_7k3m9q.md", "Latin-1 input", "non-UTF-8 input")

	revise(t, w, "bel_7k3m9q", true)
	w.Edit("belief/bel_7k3m9q.md", "claim: validate doesn't handle non-UTF-8 input", "claim: validate doesn't handle Latin-1 input")
	sync(t, w)
	n := w.Node("bel_7k3m9q")
	if r1 := n.Revision(1); r1.KeptText("claim") != "validate doesn't handle non-UTF-8 input" {
		t.Errorf("revision 1 doesn't keep the old claim: %+v", r1)
	}
	if r2 := n.Current(); r2.Rev != 2 || !r2.Breaking || r2.FP != n.Fingerprint() {
		t.Errorf("revision 2: %+v", r2)
	}
	if got := problems(t, w); got != "" {
		t.Error(got)
	}
	if stale := w.Stale(); !slices.Equal(stale, []string{"task_9w4hzd relies_on -> bel_7k3m9q"}) {
		t.Errorf("stale: %v", stale)
	}
}

func TestNonBreakingRevisionsRepinQuietly(t *testing.T) {
	w := testws.New(t, map[string]string{
		"requirement/req_5txk3a.md": "---\nid: req_5txk3a\ntitle: Unreadable files\nstatement: validate reports unreadable files\nstatus: accepted\ncreated: 2026-11-22\n---\n",
		"task/task_9w4hzd.md":       "---\nid: task_9w4hzd\ntitle: Report them\nstatus: accepted\nimplements: req_5txk3a\ncreated: 2026-11-22\n---\n",
	})
	sync(t, w)
	revise(t, w, "req_5txk3a", false)
	w.Edit("requirement/req_5txk3a.md", "reports unreadable files", "reports unreadable files clearly")
	if got := actions(sync(t, w)); !strings.Contains(got, "re-pin `implements[0]` to req_5txk3a@r2: every revision since r1 is non-breaking") {
		t.Errorf("got %s", got)
	}
	if stale := w.Stale(); len(stale) != 0 {
		t.Errorf("stale: %v", stale)
	}
	if _, err := os.Stat(w.Path("evidence")); err == nil {
		t.Error("a quiet re-pin wrote evidence")
	}
}

func TestNewRecordsGetAFirstRevision(t *testing.T) {
	w := testws.New(t, nil)
	belief, _ := registry.Load().Find("belief")
	rel, err := graph.Create(w.Root, belief, "X", time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), graph.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sync(t, w)
	n := w.Node(strings.TrimSuffix(filepath.Base(rel), ".md"))
	if r := n.Current(); r.Implicit || r.Rev != 1 || r.Date != today || r.FP != n.Fingerprint() {
		t.Errorf("revision 1: %+v", r)
	}
}

func TestSyncLeavesBrokenFilesAlone(t *testing.T) {
	broken := strings.Replace(testws.Task, "status: accepted", "status: acepted", 1)
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": broken})
	if p := sync(t, w); !slices.Equal(p.Skipped, []string{"graph/task/task_9w4hzd.md"}) {
		t.Errorf("skipped: %v", p.Skipped)
	}
	if w.Read("task/task_9w4hzd.md") != broken {
		t.Error("sync changed a file with errors")
	}
}

// A bare link to an old revision, with only non-breaking revisions after it,
// is pinned straight to the current one, so a second sync has nothing to do.
func TestSyncIsIdempotentWithNamedRevisions(t *testing.T) {
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": testws.Task})
	sync(t, w)
	revise(t, w, "bel_7k3m9q", false)
	w.Edit("belief/bel_7k3m9q.md", "non-UTF-8 input", "non-UTF-8 input, such as Latin-1")
	w.Write("task/task_2b4cdf.md", "---\nid: task_2b4cdf\ntitle: Another\nstatus: proposed\nrelies_on: [bel_7k3m9q@r1]\ncreated: 2026-11-22\n---\n")
	if got := actions(sync(t, w)); !strings.Contains(got, "pin `relies_on[0]` to bel_7k3m9q@r2, since every revision after r1 is non-breaking") {
		t.Errorf("got %s", got)
	}
	if p := sync(t, w); !p.Empty() {
		t.Errorf("a second sync did something:\n%s", actions(p))
	}
}

func TestReviseRefuses(t *testing.T) {
	w := testws.New(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"actor/who_4d9snc.md":  "---\nid: who_4d9snc\ntitle: Someone\nkind: person\ncreated: 2026-11-22\n---\n",
		"belief/bel_2wq8dn.md": strings.NewReplacer("bel_7k3m9q", "bel_2wq8dn", "created:", "revisions:\n  - {rev: 1}\n  - {rev: 3}\ncreated:").Replace(testws.Belief),
	})
	for id, want := range map[string]string{
		"bel_7k3m9q": "nothing seals revision 1 of bel_7k3m9q yet",
		"who_4d9snc": "which has no revisions",
		"bel_2wq8dn": "has errors; fix them first",
	} {
		if _, err := tryRevise(t, w, id, true); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", id, err)
		}
	}
}
