package graph

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/registry"
)

// The tests are in one file per source file, in the same order as the code.
// Each test writes the records it needs, so what it proves is on the page.

// file returns the text of a record: its ID, a title, the header lines
// given, and a body.
func file(id string, lines ...string) string {
	header := ""
	for _, l := range lines {
		header += l + "\n"
	}
	return "---\nid: " + id + "\ntitle: Record " + id + "\n" + header + "created: 2026-11-22\n---\n\nBody of " + id + ".\n"
}

var idLine = regexp.MustCompile(`(?m)^id: (\S+)$`)

// load writes records where their IDs say they belong and loads them. The
// graph must load without a diagnostic.
func load(t *testing.T, records ...string) *Graph {
	t.Helper()
	files := map[string]string{}
	for _, text := range records {
		id := idLine.FindStringSubmatch(text)[1]
		files[strings.TrimPrefix(RecordPath(registry.Load().TypeOfID(id), id), Dir+"/")] = text
	}
	g, ds := loadFiles(t, files)
	if len(ds) > 0 {
		t.Fatalf("diagnostics loading:\n%s", lines(ds))
	}
	return g
}

// loadFiles writes files at the given paths under graph/, loads them, and
// returns what loading reported.
func loadFiles(t *testing.T, files map[string]string) (*Graph, diag.List) {
	t.Helper()
	root := t.TempDir()
	for rel, text := range files {
		write(t, filepath.Join(root, Dir, rel), text)
	}
	return loadRoot(t, root)
}

func loadRoot(t *testing.T, root string) (*Graph, diag.List) {
	t.Helper()
	g, ds, err := Load(root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	ds.Sort()
	return g, ds
}

func write(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lines is diagnostics as validate prints them, one per line.
func lines(ds diag.List) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.String())
	}
	return strings.Join(out, "\n")
}

// edge returns the one edge from a node through a field, or fails.
func edge(t *testing.T, g *Graph, from, name string) *Edge {
	t.Helper()
	var found []*Edge
	for _, e := range g.Edges {
		if e.From.ID == from && e.Name == name {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d edges %s %s", len(found), from, name)
	}
	return found[0]
}

// ids lists nodes' IDs.
func ids(nodes []*Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	return out
}
