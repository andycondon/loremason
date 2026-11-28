package page

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/index"
)

var update = flag.Bool("update", false, "rewrite the golden README with what is rendered")

// ---- Path ----

func TestPath(t *testing.T) {
	if got := Path("search-redesign"); got != "initiatives/search-redesign/README.md" {
		t.Errorf("Path = %q", got)
	}
}

// ---- README ----

// readmeWorkspace is an initiative with purpose, goals, people, a body, and
// one of each kind of record the page lists.
func readmeWorkspace(t *testing.T) string {
	w := workspace(t, map[string]string{
		"actor/" + andy + ".md":      "---\nid: " + andy + "\ntitle: Andy\nkind: person\ncreated: 2026-11-22\n---\n",
		"actor/" + ci + ".md":        "---\nid: " + ci + "\ntitle: CI bot\nkind: software_agent\ncreated: 2026-11-22\n---\n",
		"initiative/" + iniA + ".md": "---\nid: " + iniA + "\ntitle: Search [redesign]\nslug: search-redesign\nstatus: active\npurpose: Make search fast and findable.\ngoals:\n  - Answer a query in under 100 ms\n  - Rank by what people open\ninvolves:\n  - {to: " + ci + ", role: interested}\n  - {to: " + andy + ", role: decides}\ncreated: 2026-11-22\n---\n\nSearch is slow today.\n",
		"design-doc/dd_c5r1tw.md":    record("dd_c5r1tw", "Index layout", "status: accepted", "part_of: "+iniA),
		"prd/prd_p1p1p1.md":          record("prd_p1p1p1", "Search PRD", "status: proposed", "part_of: "+iniA),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "Build the *index*", iniA, "progress: in_progress"),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Add the query box", iniA, "progress: done"),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "Other initiative's task", iniB),
		"activity/actv_a1a1a1.md":    record("actv_a1a1a1", "Session one", "started_at: 2026-11-22", "part_of: "+iniA),
		"activity/actv_b2b2b2.md":    record("actv_b2b2b2", "Other session", "started_at: 2026-11-22", "part_of: "+iniB),
		"finding/fnd_f1f1f1.md":      record("fnd_f1f1f1", "Queries are mostly one word", "generated_by: [actv_a1a1a1]"),
		"finding/fnd_f2f2f2.md":      record("fnd_f2f2f2", "Caching helps *most*", "generated_by: [actv_a1a1a1]"),
		"finding/fnd_f3f3f3.md":      record("fnd_f3f3f3", "Other initiative's finding", "generated_by: [actv_b2b2b2]"),
		"initiative/" + iniB + ".md": initiative(iniB, "other"),
	})
	return string(README(overview(t, w, "search-redesign")))
}

func TestREADMEGolden(t *testing.T) {
	got := readmeWorkspace(t)
	const golden = "testdata/search-redesign.md"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestREADMEHasNoDatesAndEndsWithANewline(t *testing.T) {
	got := readmeWorkspace(t)
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("should end with exactly one newline: %q", got[len(got)-10:])
	}
	if strings.Contains(got, "2026") {
		t.Error("the page shows a date")
	}
}

func TestREADMEIsStable(t *testing.T) {
	w := workspace(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "First", iniA),
	})
	first := README(overview(t, w, "alpha"))
	if second := README(overview(t, w, "alpha")); !bytes.Equal(first, second) {
		t.Errorf("two renders differ:\n%s\n%s", first, second)
	}
}

func TestREADMEOmitsEmptySections(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	got := string(README(overview(t, w, "alpha")))
	for _, h := range []string{"## Goals", "## People", "## Design docs", "## PRDs", "## Tasks"} {
		if strings.Contains(got, h) {
			t.Errorf("an empty section is shown: %s\n%s", h, got)
		}
	}
	if !strings.HasPrefix(got, "# Initiative alpha\n\n"+Marker+"\n") {
		t.Errorf("the page should start with the title and the marker:\n%s", got)
	}
}

func TestREADMEEscapesAndLinksRelatively(t *testing.T) {
	w := workspace(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "A [link] and `code`", iniA),
	})
	got := string(README(overview(t, w, "alpha")))
	if want := "- [A \\[link\\] and \\`code\\`](../../graph/task/task_t1t1t1.md): accepted, todo\n"; !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

func TestREADMEEscapesEachSegmentOfALinkPath(t *testing.T) {
	o := &index.Overview{
		Initiative: index.Summary{ID: iniA, Title: "Alpha"},
		Slug:       "alpha",
		Path:       "graph/initiative/" + iniA + ".md",
		Tasks: []index.Entry{{
			Summary: index.Summary{ID: "task_t1t1t1", Title: "T", Status: "accepted"},
			Path:    "graph/my tasks/50%#1 (a)?.md",
		}},
	}
	got := string(README(o))
	if want := "- [T](../../graph/my%20tasks/50%25%231%20%28a%29%3F.md): accepted\n"; !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

func TestREADMESortsEntriesByTitleIgnoringCase(t *testing.T) {
	w := workspace(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"task/task_t1t1t1.md":        task("task_t1t1t1", "banana", iniA),
		"task/task_t2t2t2.md":        task("task_t2t2t2", "Cherry", iniA),
		"task/task_t3t3t3.md":        task("task_t3t3t3", "apple", iniA),
	})
	got := string(README(overview(t, w, "alpha")))
	a, b, c := strings.Index(got, "[apple]"), strings.Index(got, "[banana]"), strings.Index(got, "[Cherry]")
	if a < 0 || !(a < b && b < c) {
		t.Errorf("tasks should read apple, banana, Cherry:\n%s", got)
	}
}
