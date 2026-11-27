package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests only prove the commands are wired to the index and the pages.
// What the view holds is proved in package index, and the pages in package
// page.

const (
	ini = "ini_a1b2c3"
	qid = "q_a1b2c3"
)

func rec(id, title string, lines ...string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\n" + strings.Join(lines, "\n") + "\ncreated: 2026-11-22\n---\n\nBody.\n"
}

// initiativeWorkspace has an initiative with a task in progress, a blocked
// task, a question with a leaning decision, and no pages yet.
func initiativeWorkspace(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"initiative/" + ini + ".md": rec(ini, "Search", "slug: search", "status: active", "purpose: Find things."),
		"task/task_a1a1a1.md":       rec("task_a1a1a1", "Build the index", "status: accepted", "part_of: "+ini, "progress: in_progress"),
		"task/task_b2b2b2.md":       rec("task_b2b2b2", "Ship it", "status: accepted", "part_of: "+ini, "progress: blocked", "blocked_by: [task_a1a1a1]"),
		"question/" + qid + ".md":   rec(qid, "Which engine?", "part_of: "+ini, "question: Which engine do we use?"),
		"decision/dec_c3c3c3.md":    rec("dec_c3c3c3", "Use SQLite", "status: leaning", "part_of: "+ini, "answers: "+qid),
	}
}

func TestWhere(t *testing.T) {
	newWorkspace(t, initiativeWorkspace(t))
	code, out, errOut := mason("where", "search")
	want := "ini_a1b2c3  initiative  active  Search\n" +
		"\nin progress\n  task_a1a1a1  task  accepted  Build the index\n" +
		"\nblocked\n  task_b2b2b2  task  accepted  Ship it\n    waiting on  task_a1a1a1  task  accepted  Build the index\n" +
		"\nopen questions\n  q_a1b2c3  question  -  Which engine?\n    answer waiting  dec_c3c3c3  decision  leaning  Use SQLite\n" +
		"\nneeds a re-check\n  none\n"
	if code != 0 || errOut != "" || out != want {
		t.Errorf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, errOut, out, want)
	}
}

func TestWhereAcceptsAnID(t *testing.T) {
	newWorkspace(t, initiativeWorkspace(t))
	if code, out, _ := mason("where", ini); code != 0 || !strings.HasPrefix(out, "ini_a1b2c3  initiative") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestWhereEmptySectionsSayNone(t *testing.T) {
	newWorkspace(t, map[string]string{"initiative/" + ini + ".md": rec(ini, "Search", "slug: search", "status: active")})
	_, out, _ := mason("where", "search")
	if strings.Count(out, "  none\n") != 4 {
		t.Errorf("stdout:\n%s", out)
	}
}

func TestWhereUnknownInitiative(t *testing.T) {
	newWorkspace(t, initiativeWorkspace(t))
	code, out, errOut := mason("where", "serch")
	if code != 1 || out != "" || !strings.Contains(errOut, "did you mean `search`?") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if code, _, errOut := mason("where", "task_a1a1a1"); code != 1 || !strings.Contains(errOut, "is a task, not an initiative") {
		t.Errorf("not an initiative: exit %d, stderr %q", code, errOut)
	}
}

func TestWhereAnswersWhenTheGraphHasErrors(t *testing.T) {
	records := initiativeWorkspace(t)
	records["task/task_b2b2b2.md"] = strings.Replace(records["task/task_b2b2b2.md"], "status: accepted", "status: acepted", 1)
	newWorkspace(t, records)
	code, out, errOut := mason("where", "search")
	if code != 0 || !strings.HasPrefix(out, "ini_a1b2c3  initiative") || errOut != "mason: warning: the graph has 1 error; run mason validate\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

const (
	pagePath = "initiatives/search/README.md"
	attrPath = "initiatives/.gitattributes"
)

func TestSyncWritesThePageAndAttributes(t *testing.T) {
	w := newWorkspace(t, initiativeWorkspace(t))
	code, out, errOut := mason("sync")
	if code != 0 || errOut != "" || !strings.Contains(out, pagePath+": ") || !strings.Contains(out, attrPath+": ") {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
	page, err := os.ReadFile(filepath.Join(w.Root, pagePath))
	if err != nil || !strings.HasPrefix(string(page), "# Search\n") || !strings.Contains(string(page), "[Build the index](../../graph/task/task_a1a1a1.md)") {
		t.Errorf("page %q, err %v", page, err)
	}
	attr, err := os.ReadFile(filepath.Join(w.Root, attrPath))
	if err != nil || string(attr) != "*/README.md linguist-generated=true\n" {
		t.Errorf("attributes %q, err %v", attr, err)
	}
	if code, out, _ := mason("sync"); code != 0 || out != "nothing to sync\n" {
		t.Errorf("second sync: exit %d\n%s", code, out)
	}
}

func TestSyncDryRunWritesNoPages(t *testing.T) {
	w := newWorkspace(t, initiativeWorkspace(t))
	code, out, _ := mason("sync", "--dry-run")
	if code != 0 || !strings.Contains(out, pagePath+": would ") || !strings.Contains(out, attrPath+": would ") {
		t.Errorf("exit %d\n%s", code, out)
	}
	for _, p := range []string{pagePath, attrPath} {
		if _, err := os.Stat(filepath.Join(w.Root, p)); err == nil {
			t.Errorf("a dry run wrote %s", p)
		}
	}
}

func TestSyncSkipsAHandWrittenPage(t *testing.T) {
	w := newWorkspace(t, initiativeWorkspace(t))
	path := filepath.Join(w.Root, pagePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := mason("sync")
	if code != 1 || !strings.Contains(out, pagePath+": skipped, since it is hand-written") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if got, _ := os.ReadFile(path); string(got) != "my notes\n" {
		t.Errorf("the page was overwritten: %q", got)
	}
}

func TestValidateFailsOnAStalePageUntilSync(t *testing.T) {
	w := newWorkspace(t, initiativeWorkspace(t))
	if code, out, _ := mason("validate"); code != 1 || !strings.Contains(out, pagePath+":1: error: is missing") {
		t.Errorf("before sync: exit %d\n%s", code, out)
	}
	mason("sync")
	if code, out, _ := mason("validate"); code != 0 || out != "5 records: 0 errors, 0 warnings\n" {
		t.Errorf("after sync: exit %d\n%s", code, out)
	}
	w.Edit("task/task_a1a1a1.md", "Build the index", "Build the index, fast")
	code, out, _ := mason("validate")
	if code != 1 || !strings.Contains(out, pagePath+":") || !strings.Contains(out, "is out of date") || !strings.Contains(out, "run `mason sync`") || !strings.HasSuffix(out, "1 error, 0 warnings\n") {
		t.Errorf("stale: exit %d\n%s", code, out)
	}
	mason("sync")
	if code, _, _ := mason("validate"); code != 0 {
		t.Errorf("after the second sync: exit %d", code)
	}
}
