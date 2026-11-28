package graph

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/registry"
)

var day = time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)

func TestCreateFillsTheTemplate(t *testing.T) {
	initiative, _ := registry.Load().Find("initiative")
	root := t.TempDir()
	rel, err := Create(root, initiative, "Reading files: safely", day, CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`^graph/initiative/(ini_[0-9a-z]{6})\.md$`).FindStringSubmatch(rel)
	if m == nil {
		t.Fatalf("written to %s", rel)
	}
	data, _ := os.ReadFile(filepath.Join(root, rel))
	want := "---\nid: " + m[1] + "\ntitle: 'Reading files: safely'\nslug: reading-files-safely\nstatus: active\npurpose: \"\"\ngoals: []\ninvolves: []\ncreated: 2026-11-25\n---\n\nPurpose and goals, in more detail.\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestCreateKeepsToTheTitleRule(t *testing.T) {
	belief, _ := registry.Load().Find("belief")
	root := t.TempDir()
	for _, bad := range []string{"", "   ", "a\nb", "tab\there", "bad \xff byte"} {
		if _, err := Create(root, belief, bad, day, CreateOptions{}); !errors.Is(err, ErrTitle) {
			t.Errorf("Create(%q): %v", bad, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, Dir)); err == nil {
		t.Error("a bad title still wrote to graph/")
	}
	rel, err := Create(root, belief, "  spaced  ", day, CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := loadRoot(t, root)
	if len(g.Files) != 1 || g.Files[0].Path != rel || g.Files[0].Node.Fields.Text("title") != "spaced" {
		t.Errorf("got %+v", g.Files)
	}
}

func TestSlug(t *testing.T) {
	for title, want := range map[string]string{
		"Reading files: safely": "reading-files-safely",
		"  UTF-8 -- and more ":  "utf-8-and-more",
		"日本語 notes":             "notes",
		"日本語":                   "ini-2m8vqe", // nothing usable, so the ID
	} {
		if got := slug(title, "ini_2m8vqe"); got != want {
			t.Errorf("slug(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestCreateWritesGeneratedBy(t *testing.T) {
	reg := registry.Load()
	for typ, want := range map[string]string{
		// evidence's template has an empty list to fill
		"evidence": "from: []\ngenerated_by: actv_k3m9qx\ncreated:",
		// a finding's template has none, so the line is added
		"finding": "title: Caching helps\ngenerated_by: actv_k3m9qx\ncreated:",
	} {
		ty, _ := reg.Find(typ)
		root := t.TempDir()
		rel, err := Create(root, ty, "Caching helps", day, CreateOptions{GeneratedBy: "actv_k3m9qx"})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(root, rel))
		if !strings.Contains(string(data), want) || strings.Count(string(data), "generated_by") != 1 {
			t.Errorf("%s:\n%s", typ, data)
		}
	}
}

func TestCreateWithoutOptionsAddsNoLink(t *testing.T) {
	finding, _ := registry.Load().Find("finding")
	root := t.TempDir()
	rel, _ := Create(root, finding, "Caching helps", day, CreateOptions{})
	data, _ := os.ReadFile(filepath.Join(root, rel))
	if strings.Contains(string(data), "generated_by") {
		t.Errorf("got:\n%s", data)
	}
}
