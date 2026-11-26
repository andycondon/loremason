// Package testws builds workspaces for tests: records written into a
// temporary folder, and ways to read, edit and load them. Only tests import
// it.
package testws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
)

// Version is the loremason version a workspace's config names.
const Version = "test"

// Workspace is a workspace in a temporary folder.
type Workspace struct {
	Root string
	t    testing.TB
}

// New makes a workspace holding records, keyed by their path under graph/,
// such as belief/bel_7k3m9q.md.
func New(t testing.TB, records map[string]string) *Workspace {
	t.Helper()
	w := &Workspace{Root: t.TempDir(), t: t}
	if err := os.Mkdir(filepath.Join(w.Root, ".loremason"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Root, ".loremason", "config.yaml"), []byte("version: "+Version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, text := range records {
		w.Write(rel, text)
	}
	return w
}

// Path returns the full path of a record, given its path under graph/.
func (w *Workspace) Path(rel string) string {
	return filepath.Join(w.Root, graph.Dir, filepath.FromSlash(rel))
}

// Write writes a record, given its path under graph/.
func (w *Workspace) Write(rel, text string) {
	w.t.Helper()
	p := w.Path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// Read returns a record's text, given its path under graph/.
func (w *Workspace) Read(rel string) string {
	w.t.Helper()
	data, err := os.ReadFile(w.Path(rel))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

// Edit replaces the first old in a record with new, and fails the test when
// old isn't there.
func (w *Workspace) Edit(rel, old, new string) {
	w.t.Helper()
	text := w.Read(rel)
	if !strings.Contains(text, old) {
		w.t.Fatalf("%s doesn't contain %q", rel, old)
	}
	w.Write(rel, strings.Replace(text, old, new, 1))
}

// Load loads the graph. Diagnostics are the checks' business, so only an
// error loading fails the test.
func (w *Workspace) Load() *graph.Graph {
	w.t.Helper()
	g, _, err := graph.Load(w.Root, registry.Load())
	if err != nil {
		w.t.Fatal(err)
	}
	return g
}

// Node loads the graph and returns a node, failing the test when it isn't
// there.
func (w *Workspace) Node(id string) *graph.Node {
	w.t.Helper()
	n := w.Load().Node(id)
	if n == nil {
		w.t.Fatalf("no node %s", id)
	}
	return n
}

// Stale lists the stale links, as "from field -> target".
func (w *Workspace) Stale() []string {
	w.t.Helper()
	var stale []string
	for _, e := range w.Load().Edges {
		if e.Stale() {
			stale = append(stale, e.From.ID+" "+e.Name+" -> "+e.Target.ID)
		}
	}
	return stale
}

// Belief is a belief that nothing has revised yet.
const Belief = `---
id: bel_7k3m9q
title: validate and encodings
claim: validate doesn't handle non-UTF-8 input
status: held
asserted_by: agent
created: 2026-11-22
---

Both failures are on files that aren't UTF-8.
`

// BeliefFP is Belief's fingerprint.
const BeliefFP = "fp1:536221e3130c"

// Task relies on Belief, and is written the way a person writes by hand:
// comments, odd spacing, a folded block, an indented line in the body.
const Task = `---
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
