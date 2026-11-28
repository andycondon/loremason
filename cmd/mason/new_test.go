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
			m := regexp.MustCompile(`generated_by: (actv_[0-9a-z]{6})`).FindStringSubmatch(string(data))
			if m == nil || strings.Contains(string(data), "generated_by: [") {
				t.Fatalf("not attached as one value:\n%s", data)
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

func TestNewRejectsABadSessionEnvironment(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"LOREMASON_ACTOR":   {"LOREMASON_ACTOR": "a], status: x"},
		"LOREMASON_SESSION": {"LOREMASON_SESSION": "a\nb"},
	} {
		w := sessionWorkspace(t)
		for k, v := range env {
			t.Setenv(k, v)
		}
		if code, _, errOut := mason("new", "finding", "--title", "X"); code != 2 || !strings.Contains(errOut, name) {
			t.Errorf("%s: exit %d, stderr %q", name, code, errOut)
		}
		if _, err := os.Stat(filepath.Join(w.Root, "graph", "activity")); err == nil {
			t.Errorf("%s: an activity was written", name)
		}
	}
}

func TestNewBlankSessionIsUnset(t *testing.T) {
	newWorkspace(t, nil)
	t.Setenv("LOREMASON_SESSION", "  ")
	code, _, errOut := mason("new", "finding", "--title", "X")
	if code != 0 || !strings.Contains(errOut, "isn't attached to a session") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestNewBadTitleWritesNoActivity(t *testing.T) {
	w := sessionWorkspace(t)
	if code, _, errOut := mason("new", "finding", "--title", "a\nb"); code != 2 || !strings.Contains(errOut, "one line") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(w.Root, "graph", "activity")); err == nil {
		t.Error("an activity was written")
	}
}

func TestNewSaysWhenTheActivitysInitiativeWins(t *testing.T) {
	w := sessionWorkspace(t)
	w.Write("initiative/ini_d4e5f6.md", "---\nid: ini_d4e5f6\ntitle: Other\nslug: other\nstatus: active\ncreated: 2026-11-22\n---\n\nBody.\n")
	if code, _, errOut := mason("new", "finding", "--title", "One"); code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	t.Setenv("LOREMASON_INITIATIVE", "other")
	code, out, errOut := mason("new", "finding", "--title", "Two")
	if code != 0 || strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, "belongs to search already") || !strings.Contains(errOut, "LOREMASON_INITIATIVE=other") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if data, _ := os.ReadFile(strings.TrimSuffix(out, "\n")); !strings.Contains(string(data), "generated_by") {
		t.Errorf("not attached:\n%s", data)
	}
}

// One session's records share one activity, and it all validates.
func TestNewOneSessionOneActivity(t *testing.T) {
	w := sessionWorkspace(t)
	var paths []string
	for _, a := range [][]string{{"finding", "One"}, {"finding", "Two"}, {"evidence", "Quote"}} {
		code, out, errOut := mason("new", a[0], "--title", a[1])
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, errOut)
		}
		paths = append(paths, strings.TrimSuffix(out, "\n"))
	}
	acts, _ := filepath.Glob(filepath.Join(w.Root, "graph", "activity", "*.md"))
	if len(acts) != 1 {
		t.Fatalf("activities: %v", acts)
	}
	id := strings.TrimSuffix(filepath.Base(acts[0]), ".md")
	for _, p := range paths {
		if data, _ := os.ReadFile(p); !strings.Contains(string(data), "generated_by: "+id) {
			t.Errorf("%s isn't linked to %s:\n%s", p, id, data)
		}
	}
	mason("sync")
	if code, out, errOut := mason("validate"); code != 0 {
		t.Errorf("validate: exit %d\n%s%s", code, out, errOut)
	}
}

func TestNewArtifactIsAPage(t *testing.T) {
	newWorkspace(t, nil)
	code, out, errOut := mason("new", "artifact", "--title", "A page")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if data, _ := os.ReadFile(strings.TrimSuffix(out, "\n")); !strings.Contains(string(data), "kind: page\n") {
		t.Errorf("got:\n%s", data)
	}
}

func TestValidateRejectsAnArtifactOfKindDocument(t *testing.T) {
	newWorkspace(t, map[string]string{"artifact/art_k3m9qx.md": "---\nid: art_k3m9qx\ntitle: A doc\nkind: document\ncreated: 2026-11-22\n---\n"})
	if code, out, errOut := mason("validate"); code != 1 || !strings.Contains(out+errOut, "kind") {
		t.Errorf("exit %d\n%s%s", code, out, errOut)
	}
}

// Both links are fine on a document: one says which activity produced it,
// the other who it is credited to.
func TestValidateAcceptsGeneratedByAndAttributedTo(t *testing.T) {
	newWorkspace(t, map[string]string{
		"activity/actv_k3m9qx.md": "---\nid: actv_k3m9qx\ntitle: Session\nsession: s\nstarted_at: 2026-11-22\ncreated: 2026-11-22\n---\n",
		"actor/who_4d9snc.md":     "---\nid: who_4d9snc\ntitle: Someone\nkind: person\ncreated: 2026-11-22\n---\n",
		"finding/fnd_k3m9qx.md":   "---\nid: fnd_k3m9qx\ntitle: F\ngenerated_by: actv_k3m9qx\nattributed_to: [who_4d9snc]\ncreated: 2026-11-22\n---\n\nText.\n",
		"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: D\nstatus: proposed\ngenerated_by: actv_k3m9qx\nattributed_to: [who_4d9snc]\ncreated: 2026-11-22\n---\n\nText.\n",
	})
	mason("sync")
	if code, out, errOut := mason("validate"); code != 0 {
		t.Errorf("exit %d\n%s%s", code, out, errOut)
	}
}

func TestREADMEListsFindingsOfEverySessionOfTheInitiative(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"initiative/" + sessionIni + ".md": "---\nid: " + sessionIni + "\ntitle: Search\nslug: search\nstatus: active\ncreated: 2026-11-22\n---\n\nBody.\n",
		"activity/actv_k3m9qx.md":          "---\nid: actv_k3m9qx\ntitle: One\nsession: a\nstarted_at: 2026-11-22\npart_of: " + sessionIni + "\ncreated: 2026-11-22\n---\n",
		"activity/actv_p4n2xa.md":          "---\nid: actv_p4n2xa\ntitle: Two\nsession: b\nstarted_at: 2026-11-23\npart_of: " + sessionIni + "\ncreated: 2026-11-23\n---\n",
		"finding/fnd_a1a1a1.md":            "---\nid: fnd_a1a1a1\ntitle: Alpha\ngenerated_by: actv_k3m9qx\ncreated: 2026-11-22\n---\n\nA.\n",
		"finding/fnd_b2b2b2.md":            "---\nid: fnd_b2b2b2\ntitle: Beta\ngenerated_by: actv_k3m9qx\ncreated: 2026-11-22\n---\n\nB.\n",
		"finding/fnd_c3c3c3.md":            "---\nid: fnd_c3c3c3\ntitle: Gamma\ngenerated_by: actv_p4n2xa\ncreated: 2026-11-23\n---\n\nC.\n",
	})
	if code, _, errOut := mason("sync"); code != 0 {
		t.Fatalf("sync: exit %d, %s", code, errOut)
	}
	data, err := os.ReadFile(filepath.Join(w.Root, "initiatives", "search", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(string(data), "["+title+"](") {
			t.Errorf("%s missing:\n%s", title, data)
		}
	}
}
