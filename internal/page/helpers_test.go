package page

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// The tests are in one file per source file, in the same order as the code.
// Each test writes the records it needs, so what it proves is on the page.

const (
	iniA = "ini_a1b2c3"
	iniB = "ini_d4e5f6"
	andy = "who_a1a1a1"
	ci   = "who_b2b2b2"
)

// record returns the text of a record: its ID, a title, the header lines
// given, and a body.
func record(id, title string, lines ...string) string {
	header := ""
	for _, l := range lines {
		header += l + "\n"
	}
	return "---\nid: " + id + "\ntitle: " + title + "\n" + header + "created: 2026-11-22\n---\n\nBody of " + id + ".\n"
}

// initiative is an initiative record with a slug and the lines given.
func initiative(id, slug string, lines ...string) string {
	return record(id, "Initiative "+slug, append([]string{"slug: " + slug, "status: active"}, lines...)...)
}

// task is a task that is part of an initiative.
func task(id, title, ini string, lines ...string) string {
	return record(id, title, append([]string{"status: accepted", "part_of: " + ini}, lines...)...)
}

// workspace writes records, keyed by their path under graph/.
func workspace(t *testing.T, records map[string]string) *testws.Workspace {
	t.Helper()
	return testws.New(t, records)
}

var clock = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// edit replaces text in a record and gives it a new modification time, so
// the index sees the change.
func edit(t *testing.T, w *testws.Workspace, rel, old, new string) {
	t.Helper()
	w.Edit(rel, old, new)
	stamp(t, w, rel)
}

func stamp(t *testing.T, w *testws.Workspace, rel string) {
	t.Helper()
	clock = clock.Add(time.Minute)
	if err := os.Chtimes(w.Path(rel), clock, clock); err != nil {
		t.Fatal(err)
	}
}

// plan opens the index of the workspace and plans its pages.
func plan(t *testing.T, w *testws.Workspace) (*Changes, diag.List) {
	t.Helper()
	x, err := index.Open(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	p, ds, err := Plan(w.Root, x)
	if err != nil {
		t.Fatal(err)
	}
	return p, ds
}

// syncPages plans and applies, and checks that a second plan is empty.
func syncPages(t *testing.T, w *testws.Workspace) {
	t.Helper()
	p, _ := plan(t, w)
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if p, ds := plan(t, w); len(p.Actions) != 0 || len(p.Skipped) != 0 || len(ds) != 0 {
		t.Fatalf("a second plan should be empty: %+v, %v", p, ds)
	}
}

func read(t *testing.T, w *testws.Workspace, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func write(t *testing.T, w *testws.Workspace, rel, text string) {
	t.Helper()
	p := filepath.Join(w.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(w *testws.Workspace, rel string) bool {
	_, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(rel)))
	return err == nil
}

// paths lists where diagnostics are, as "path" strings joined by commas.
func paths(ds diag.List) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Path)
	}
	return strings.Join(out, ",")
}

// overview opens the index and returns the overview of a slug.
func overview(t *testing.T, w *testws.Workspace, slug string) *index.Overview {
	t.Helper()
	x, err := index.Open(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	o, err := x.Overview(slug)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
