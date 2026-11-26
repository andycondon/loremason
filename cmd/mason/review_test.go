package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for what the milestone 2 review found, one per finding.

func TestEmptyRevisionsListIsRevisionOne(t *testing.T) {
	root := newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(beliefRecord, "created:", "revisions: []\ncreated:", 1),
		"task/task_9w4hzd.md":  taskRecord,
	})
	must(t, "sync") // panicked before
	if n := graphNode(t, root, "bel_7k3m9q"); len(n.Revisions) != 1 || n.Current().Implicit {
		t.Errorf("revisions: %+v", n.Revisions)
	}
	must(t, "revise", "bel_7k3m9q", "--breaking")
}

func TestCommentAfterTheLastLinkAppearsOnce(t *testing.T) {
	task := strings.Replace(taskRecord, "relies_on: [bel_7k3m9q]   # the one belief this rests on\n", "relies_on:\n  - bel_7k3m9q\n  # a note after the list\n", 1)
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": task})
	must(t, "sync")
	must(t, "revise", "bel_7k3m9q", "--non-breaking")
	must(t, "sync") // re-pins, rewriting relies_on a second time
	if got := read(t, root, "task/task_9w4hzd.md"); strings.Count(got, "# a note after the list") != 1 || strings.Contains(got, ",}") {
		t.Errorf("got:\n%s", got)
	}
}

func TestReviseWarnsWhenNoCommitHoldsTheOldText(t *testing.T) {
	newWorkspace(t, map[string]string{
		"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: Reading files safely\nstatus: accepted\ncreated: 2026-11-22\n---\n\nThe design.\n",
		"task/task_9w4hzd.md":     "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nderived_from: dd_c5r1tw\ncreated: 2026-11-22\n---\n",
	})
	must(t, "sync")
	code, _, errOut := mason("revise", "dd_c5r1tw", "--non-breaking") // not in git
	if code != 0 || !strings.Contains(errOut, "warning: graph/design-doc/dd_c5r1tw.md: no commit holds revision 1's text") {
		t.Errorf("exit %d, %s", code, errOut)
	}
}

func TestReviseRefusesFilesWithErrors(t *testing.T) {
	newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(beliefRecord, "created:", "revisions:\n  - {rev: 1}\n  - {rev: 3}\ncreated:", 1),
		"task/task_9w4hzd.md":  "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nrelies_on:\n  - {to: bel_7k3m9q, rev: 3, fp: \"fp1:000000000000\"}\ncreated: 2026-11-22\n---\n",
	})
	if code, _, errOut := mason("revise", "bel_7k3m9q", "--breaking"); code != 1 || !strings.Contains(errOut, "has errors; fix them first") {
		t.Errorf("exit %d, %s", code, errOut)
	}
}

func TestPinnedBeforeSyncIsResolvedBySync(t *testing.T) {
	// The form part 3, section 2 shows: a pin to a record that has no
	// revisions list yet.
	root := newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": beliefRecord,
		"task/task_9w4hzd.md":  "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nrelies_on:\n  - {to: bel_7k3m9q, rev: 1, fp: \"fp1:536221e3130c\"}\ncreated: 2026-11-22\n---\n",
	})
	if out := must(t, "validate"); !strings.Contains(out, "warning: revision 1 is pinned but isn't written down yet; run `mason sync`") {
		t.Error(out)
	}
	must(t, "sync")
	if out := must(t, "validate"); !strings.HasSuffix(out, "0 errors, 0 warnings\n") {
		t.Error(out)
	}
	if stale := staleLinks(t, root); len(stale) != 0 {
		t.Errorf("stale: %v", stale)
	}
}

func TestAPinMustCarryItsRevisionsFingerprint(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord, "task/task_9w4hzd.md": taskRecord})
	must(t, "sync")
	edit(t, root, "task/task_9w4hzd.md", "fp1:536221e3130c", "fp1:999999999999")
	if code, out, _ := mason("validate"); code != 1 || !strings.Contains(out, "the pin's fingerprint isn't the one recorded for revision 1 of bel_7k3m9q") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestKeptWordingLeavesCommentsBehind(t *testing.T) {
	root := newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(beliefRecord, "non-UTF-8 input", "non-UTF-8 input   # first seen on a Latin-1 file", 1),
		"task/task_9w4hzd.md":  taskRecord,
	})
	must(t, "sync")
	must(t, "revise", "bel_7k3m9q", "--breaking")
	revisions := read(t, root, "belief/bel_7k3m9q.md")
	revisions = revisions[strings.Index(revisions, "revisions:"):]
	if strings.Contains(revisions, "first seen") {
		t.Errorf("the comment was kept with the wording:\n%s", revisions)
	}
}

func TestSyncEditsTheFileASymlinkPointsAt(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": beliefRecord})
	real := filepath.Join(t.TempDir(), "task.md")
	os.WriteFile(real, []byte(taskRecord), 0o600)
	link := filepath.Join(root, "graph", "task", "task_9w4hzd.md")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	must(t, "sync")
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced")
	}
	data, _ := os.ReadFile(real)
	if st, _ := os.Stat(real); !strings.Contains(string(data), "rev: 1") || st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v:\n%s", st.Mode(), data)
	}
}

func TestByteOrderMarkFileSyncs(t *testing.T) {
	root := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": "\ufeff" + beliefRecord, "task/task_9w4hzd.md": taskRecord})
	must(t, "sync")
	if got := read(t, root, "belief/bel_7k3m9q.md"); !strings.HasPrefix(got, "\ufeff---\n") || !strings.Contains(got, "revisions:") {
		t.Errorf("got %q", got)
	}
}
