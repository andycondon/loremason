package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

func TestNew(t *testing.T) {
	w := newWorkspace(t, nil)
	code, out, errOut := mason("new", "design-doc", "--title", "Reading files safely")
	path := strings.TrimSuffix(out, "\n")
	if code != 0 || errOut != "" || filepath.Dir(path) != filepath.Join(w.Root, "graph", "design-doc") {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "title: Reading files safely\n") {
		t.Errorf("%v\n%s", err, data)
	}
}

func TestNewMistakes(t *testing.T) {
	w := newWorkspace(t, nil)
	for want, args := range map[string][]string{
		"did you mean `belief`?":           {"new", "beleif", "--title", "X"},
		"needs --title":                    {"new", "belief"},
		"a title must be one line of text": {"new", "belief", "--title", "a\nb"},
	} {
		if code, _, errOut := mason(args...); code != 2 || !strings.Contains(errOut, want) {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
	if _, err := os.Stat(w.Path("")); err == nil {
		t.Error("a mistake still wrote to graph/")
	}
}

const sessionIni = "ini_a1b2c3"

func sessionWorkspace(t *testing.T) *testws.Workspace {
	t.Helper()
	w := newWorkspace(t, map[string]string{
		"initiative/" + sessionIni + ".md": "---\nid: " + sessionIni + "\ntitle: Search\nslug: search\nstatus: active\ncreated: 2026-11-22\n---\n\nBody.\n",
	})
	t.Setenv("LOREMASON_SESSION", "sess-1")
	t.Setenv("LOREMASON_INITIATIVE", "search")
	return w
}

func TestNewAttachesToTheSession(t *testing.T) {
	for _, typ := range []string{"finding", "evidence"} {
		t.Run(typ, func(t *testing.T) {
			sessionWorkspace(t)
			code, out, errOut := mason("new", typ, "--title", "Caching helps")
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			data, err := os.ReadFile(strings.TrimSuffix(out, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			m := regexp.MustCompile(`generated_by: \[(actv_[0-9a-z]{6})\]`).FindStringSubmatch(string(data))
			if m == nil {
				t.Fatalf("not attached:\n%s", data)
			}
			actv := filepath.Join(filepath.Dir(filepath.Dir(strings.TrimSuffix(out, "\n"))), "activity", m[1]+".md")
			if a, err := os.ReadFile(actv); err != nil || !strings.Contains(string(a), "part_of: "+sessionIni) || !strings.Contains(string(a), "session: sess-1") {
				t.Errorf("%v\n%s", err, a)
			}
			// A second record joins the same activity.
			_, out2, _ := mason("new", typ, "--title", "More")
			if data2, _ := os.ReadFile(strings.TrimSuffix(out2, "\n")); !strings.Contains(string(data2), m[1]) {
				t.Errorf("second record:\n%s", data2)
			}
			mason("sync")
			if code, out, errOut := mason("validate"); code != 0 {
				t.Errorf("validate: exit %d\n%s%s", code, out, errOut)
			}
		})
	}
}

func TestNewLeavesOtherTypesUnattached(t *testing.T) {
	w := newWorkspace(t, nil)
	t.Setenv("LOREMASON_SESSION", "sess-1")
	code, out, errOut := mason("new", "design-doc", "--title", "Reading files safely")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if data, _ := os.ReadFile(strings.TrimSuffix(out, "\n")); strings.Contains(string(data), "generated_by") {
		t.Errorf("attached:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(w.Root, "graph", "activity")); err == nil {
		t.Error("an activity was written")
	}
}

func TestNewWithoutASessionSaysSo(t *testing.T) {
	newWorkspace(t, nil)
	t.Setenv("LOREMASON_SESSION", "")
	code, out, errOut := mason("new", "finding", "--title", "Caching helps")
	if code != 0 || strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, "isn't attached to a session") || !strings.Contains(errOut, "LOREMASON_SESSION") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if data, _ := os.ReadFile(strings.TrimSuffix(out, "\n")); strings.Contains(string(data), "generated_by") {
		t.Errorf("attached:\n%s", data)
	}
	if code, out, errOut := mason("validate"); code != 0 || strings.Contains(out+errOut, "generated_by") || strings.Contains(out+errOut, "session") {
		t.Errorf("validate: exit %d\n%s%s", code, out, errOut)
	}
}

func TestNewSessionNeedsAnInitiative(t *testing.T) {
	sessionWorkspace(t)
	t.Setenv("LOREMASON_INITIATIVE", "")
	if code, _, errOut := mason("new", "finding", "--title", "X"); code != 2 || !strings.Contains(errOut, "LOREMASON_INITIATIVE") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	t.Setenv("LOREMASON_INITIATIVE", "serch")
	if code, _, errOut := mason("new", "finding", "--title", "X"); code == 0 || !strings.Contains(errOut, "did you mean `search`?") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestNewFindingIsListedInTheInitiativesREADME(t *testing.T) {
	w := sessionWorkspace(t)
	if code, _, errOut := mason("new", "finding", "--title", "Caching helps"); code != 0 {
		t.Fatalf("new: exit %d, %s", code, errOut)
	}
	if code, _, errOut := mason("sync"); code != 0 {
		t.Fatalf("sync: exit %d, %s", code, errOut)
	}
	data, err := os.ReadFile(filepath.Join(w.Root, "initiatives", "search", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n## Findings\n\n- [Caching helps](../../graph/finding/fnd_") {
		t.Errorf("README:\n%s", data)
	}
}
