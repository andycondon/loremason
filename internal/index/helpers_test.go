package index

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// The tests are in one file per source file, in the same order as the code.
// Each test writes the records it needs, so what it proves is on the page.

// record returns the text of a record: its ID, a title, the header lines
// given, and a body.
func record(id, title string, lines ...string) string {
	header := ""
	for _, l := range lines {
		header += l + "\n"
	}
	return "---\nid: " + id + "\ntitle: " + title + "\n" + header + "created: 2026-11-22\n---\n\nBody of " + id + ".\n"
}

// newWS writes records, keyed by their path under graph/.
func newWS(t *testing.T, records map[string]string) *testws.Workspace {
	t.Helper()
	return testws.New(t, records)
}

// open opens the index of a workspace, closing it when the test ends.
func open(t *testing.T, w *testws.Workspace) *Index {
	t.Helper()
	x, err := Open(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { x.Close() })
	return x
}

// clock gives each call a later time than the last, so an edit is never
// within the same clock tick as the file's previous write.
var clock = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// stamp sets a file's modification time to the next tick.
func stamp(t *testing.T, w *testws.Workspace, rel string) {
	t.Helper()
	clock = clock.Add(time.Minute)
	if err := os.Chtimes(w.Path(rel), clock, clock); err != nil {
		t.Fatal(err)
	}
}

// put writes a record and gives it a new modification time.
func put(t *testing.T, w *testws.Workspace, rel, text string) {
	t.Helper()
	w.Write(rel, text)
	stamp(t, w, rel)
}

// edit replaces text in a record and gives it a new modification time.
func edit(t *testing.T, w *testws.Workspace, rel, old, new string) {
	t.Helper()
	w.Edit(rel, old, new)
	stamp(t, w, rel)
}

// indexPath is where a workspace's index is.
func indexPath(w *testws.Workspace) string {
	return filepath.Join(w.Root, ".loremason", "index.db")
}

// workedExample copies the worked example's records into a workspace.
func workedExample(t *testing.T) *testws.Workspace {
	t.Helper()
	base := filepath.Join("..", "..", "testdata", "worked-example", "graph")
	records := map[string]string{}
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		records[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return newWS(t, records)
}

// ids lists the IDs of summaries, in order.
func ids(ss []Summary) string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return strings.Join(out, " ")
}

// requirement is a requirement with a second revision, breaking or not.
func requirement(id, change string) string {
	return `---
id: ` + id + `
title: Unreadable files
statement: validate reports unreadable files clearly
status: accepted
revisions:
  - {rev: 1, date: 2026-11-22, fp: "fp1:000000000001", statement: a}
  - {rev: 2, date: 2026-11-23, fp: "fp1:000000000002", change: ` + change + `, statement: b}
created: 2026-11-22
---
`
}

func itoa(n int) string { return strconv.Itoa(n) }

// removeIndex deletes a workspace's index.
func removeIndex(w *testws.Workspace) error { return os.Remove(indexPath(w)) }

// loadGraph loads the graph of the workspace at root.
func loadGraph(tb testing.TB, root string) *graph.Graph {
	tb.Helper()
	g, _, err := graph.Load(root, registry.Load())
	if err != nil {
		tb.Fatal(err)
	}
	return g
}

// removeAll deletes the index files of the workspace at root, so the next
// open builds it.
func removeAll(tb testing.TB, root string) {
	tb.Helper()
	files, err := filepath.Glob(filepath.Join(root, ".loremason", "index.db*"))
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			tb.Fatal(err)
		}
	}
}

// leftovers lists the temporary and journal files beside a workspace's
// index.
func leftovers(t *testing.T, w *testws.Workspace) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(w.Root, ".loremason", "index.db*"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if f != indexPath(w) {
			out = append(out, filepath.Base(f))
		}
	}
	return out
}

// TestMain removes the generated workspace when the tests end.
func TestMain(m *testing.M) {
	code := m.Run()
	if scaleRoot != "" {
		os.RemoveAll(scaleRoot)
	}
	os.Exit(code)
}

// scaleDir makes the folder for the generated workspace.
func scaleDir() string {
	d, err := os.MkdirTemp("", "index-scale-")
	if err != nil {
		panic(err)
	}
	if err := os.Mkdir(filepath.Join(d, ".loremason"), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(d, ".loremason", "config.yaml"), []byte("version: "+testws.Version+"\n"), 0o644); err != nil {
		panic(err)
	}
	return d
}
