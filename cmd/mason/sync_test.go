package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
)

// newWorkspace writes records (by path under graph/) into a fresh
// workspace, and points mason at it.
func newWorkspace(t *testing.T, records map[string]string) string {
	t.Helper()
	root := emptyWorkspace(t)
	for rel, text := range records {
		write(t, root, rel, text)
	}
	return root
}

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, "graph", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "graph", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// edit replaces text in a record, and fails when it isn't there.
func edit(t *testing.T, root, rel, old, new string) {
	t.Helper()
	text := read(t, root, rel)
	if !strings.Contains(text, old) {
		t.Fatalf("%s doesn't contain %q", rel, old)
	}
	write(t, root, rel, strings.Replace(text, old, new, 1))
}

// must runs mason and fails unless it exits 0.
func must(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := mason(args...)
	if code != 0 {
		t.Fatalf("mason %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

// staleLinks loads the workspace and lists its stale links as
// "from field -> target".
func staleLinks(t *testing.T, root string) []string {
	t.Helper()
	g, _, err := graph.Load(root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	var stale []string
	for _, e := range g.Edges {
		if e.Stale() {
			stale = append(stale, e.From.ID+" "+e.Name+" -> "+e.Target.ID)
		}
	}
	return stale
}

const beliefRecord = `---
id: bel_7k3m9q
title: validate and encodings
claim: validate doesn't handle non-UTF-8 input
status: held
asserted_by: agent
created: 2026-11-22
---

Both failures are on files that aren't UTF-8.
`

// A task a person wrote with care: comments, odd spacing, a folded block,
// trailing spaces in the body.
const taskRecord = `---
id: task_9w4hzd
title: "Report unreadable files clearly: name the file"
status: accepted   # agreed 2026-11-22
relies_on: [bel_7k3m9q]   # the one belief this rests on
created: 2026-11-22
extra:
  confidence: 0.7   # experiment
  notes: >
    A folded block that someone wrote by hand,
    spread over two lines.
---

## Outcome
validate names any file it can't read.

  Indented line.
`

func TestSyncPinsLinksAndTouchesNothingElse(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": taskRecord})
	must(t, "sync")
	got := read(t, root, "task/task_9w4hzd.md")
	fp := graphNode(t, root, "bel_7k3m9q").Fingerprint().String()
	pinned := "relies_on: # the one belief this rests on\n  - {to: bel_7k3m9q, rev: 1, fp: \"" + fp + "\"}\n"
	if !strings.Contains(got, pinned) {
		t.Fatalf("the link isn't pinned as expected:\n%s", got)
	}
	// Everything but relies_on and the new revisions is byte-for-byte the same.
	rest := got[:strings.Index(got, "relies_on:")] + got[strings.Index(got, "created:"):]
	want := taskRecord[:strings.Index(taskRecord, "relies_on:")] + taskRecord[strings.Index(taskRecord, "created:"):]
	if rest != want {
		t.Errorf("lines nobody touched changed:\n%s", got)
	}
	if out := must(t, "sync"); out != "nothing to sync\n" {
		t.Errorf("a second sync did something:\n%s", out)
	}
	if out := must(t, "validate"); !strings.HasSuffix(out, "0 errors, 0 warnings\n") {
		t.Error(out)
	}
}

func TestDraftsStayCheap(t *testing.T) {
	root := newWorkspace(t, map[string]string{"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: Reading files safely\nstatus: proposed\ncreated: 2026-11-22\n---\n\nFirst draft.\n"})
	must(t, "sync")
	edit(t, root, "design-doc/dd_c5r1tw.md", "First draft.", "Second draft, much better.")
	if out := must(t, "sync"); !strings.Contains(out, "refresh the fingerprint of revision 1, which nothing pins yet") {
		t.Errorf("got %s", out)
	}
	if n := graphNode(t, root, "dd_c5r1tw"); len(n.Revisions) != 1 || n.Current().FP != n.Fingerprint() {
		t.Errorf("revisions: %+v", n.Revisions)
	}
}

func TestReviseThenEdit(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": taskRecord})
	must(t, "sync")
	if code, out, _ := mason("validate"); code != 0 {
		t.Fatal(out)
	}

	// Editing a pinned claim without revising is caught.
	edit(t, root, "belief/bel_7k3m9q.md", "non-UTF-8 input", "Latin-1 input")
	if code, out, _ := mason("validate"); code != 1 || !strings.Contains(out, "run `mason revise bel_7k3m9q --breaking` (or `--non-breaking`)") {
		t.Errorf("exit %d\n%s", code, out)
	}
	edit(t, root, "belief/bel_7k3m9q.md", "Latin-1 input", "non-UTF-8 input")

	must(t, "revise", "bel_7k3m9q", "--breaking")
	edit(t, root, "belief/bel_7k3m9q.md", "claim: validate doesn't handle non-UTF-8 input", "claim: validate doesn't handle Latin-1 input")
	must(t, "sync")
	n := graphNode(t, root, "bel_7k3m9q")
	if r1 := n.Revision(1); r1.Old.Text("claim") != "validate doesn't handle non-UTF-8 input" {
		t.Errorf("revision 1 doesn't keep the old claim: %+v", r1)
	}
	if r2 := n.Current(); r2.Rev != 2 || !r2.Breaking || r2.FP != n.Fingerprint() {
		t.Errorf("revision 2: %+v", r2)
	}
	if out := must(t, "validate"); !strings.Contains(out, "0 errors") {
		t.Error(out)
	}
	if stale := staleLinks(t, root); len(stale) != 1 || stale[0] != "task_9w4hzd relies_on -> bel_7k3m9q" {
		t.Errorf("stale: %v", stale)
	}
}

func TestNonBreakingRevisionsRepinQuietly(t *testing.T) {
	root := newWorkspace(t, map[string]string{
		"requirement/req_5txk3a.md": "---\nid: req_5txk3a\ntitle: Unreadable files\nstatement: validate reports unreadable files\nstatus: accepted\ncreated: 2026-11-22\n---\n",
		"task/task_9w4hzd.md":       "---\nid: task_9w4hzd\ntitle: Report them\nstatus: accepted\nimplements: req_5txk3a\ncreated: 2026-11-22\n---\n",
	})
	must(t, "sync")
	must(t, "revise", "req_5txk3a", "--non-breaking")
	edit(t, root, "requirement/req_5txk3a.md", "reports unreadable files", "reports unreadable files clearly")
	out := must(t, "sync")
	if !strings.Contains(out, "re-pin `implements[0]` to req_5txk3a@r2: every revision since r1 is non-breaking") {
		t.Errorf("got %s", out)
	}
	if stale := staleLinks(t, root); len(stale) != 0 {
		t.Errorf("stale: %v", stale)
	}
	if _, err := os.Stat(filepath.Join(root, "graph", "evidence")); err == nil {
		t.Error("a quiet re-pin wrote evidence")
	}
}

func TestNewRecordsGetAFirstRevision(t *testing.T) {
	root := emptyWorkspace(t)
	path := strings.TrimSpace(must(t, "new", "belief", "--title", "X"))
	must(t, "sync")
	id := strings.TrimSuffix(filepath.Base(path), ".md")
	n := graphNode(t, root, id)
	if r := n.Current(); r.Implicit || r.Rev != 1 || r.Date != today() || r.FP != n.Fingerprint() {
		t.Errorf("revision 1: %+v", r)
	}
}

func TestSyncLeavesBrokenFilesAlone(t *testing.T) {
	broken := strings.Replace(taskRecord, "status: accepted", "status: acepted", 1)
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": broken})
	code, out, _ := mason("sync")
	if code != 1 || !strings.Contains(out, "graph/task/task_9w4hzd.md: skipped, since it has errors; see `mason validate`") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if read(t, root, "task/task_9w4hzd.md") != broken {
		t.Error("sync changed a file with errors")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": taskRecord})
	out := must(t, "sync", "--dry-run")
	if !strings.Contains(out, "would pin `relies_on[0]` to bel_7k3m9q@r1") || read(t, root, "task/task_9w4hzd.md") != taskRecord {
		t.Errorf("got %s", out)
	}
}

func TestReviseMistakes(t *testing.T) {
	newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "actor/who_4d9snc.md": "---\nid: who_4d9snc\ntitle: Someone\nkind: person\ncreated: 2026-11-22\n---\n"})
	for want, args := range map[string][]string{
		"say whether the change is --breaking": {"revise", "bel_7k3m9q"},
		"takes one record ID":                  {"revise", "--breaking"},
	} {
		if code, _, errOut := mason(args...); code != 2 || !strings.Contains(errOut, want) {
			t.Errorf("%v: exit %d, %s", args, code, errOut)
		}
	}
	for want, args := range map[string][]string{
		"nothing pins revision 1 of bel_7k3m9q yet": {"revise", "bel_7k3m9q", "--breaking"},
		"which has no revisions":                    {"revise", "who_4d9snc", "--breaking"},
		"no record is called `bel_nope00`":          {"revise", "bel_nope00", "--breaking"},
	} {
		if code, _, errOut := mason(args...); code != 1 || !strings.Contains(errOut, want) {
			t.Errorf("%v: exit %d, %s", args, code, errOut)
		}
	}
}

func graphNode(t *testing.T, root, id string) *graph.Node {
	t.Helper()
	g, _, err := graph.Load(root, registry.Load())
	if err != nil || g.Node(id) == nil {
		t.Fatalf("loading %s: %v", id, err)
	}
	return g.Node(id)
}
