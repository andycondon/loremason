package pinning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// Regression tests for what the milestone 2 review found.

func TestEmptyRevisionsListIsRevisionOne(t *testing.T) {
	w := testws.New(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(testws.Belief, "created:", "revisions: []\ncreated:", 1),
		"task/task_9w4hzd.md":  testws.Task,
	})
	sync(t, w) // panicked before
	if n := w.Node("bel_7k3m9q"); len(n.Revisions) != 1 || n.Current().Implicit {
		t.Errorf("revisions: %+v", n.Revisions)
	}
	revise(t, w, "bel_7k3m9q", true)
}

func TestCommentAfterTheLastLinkAppearsOnce(t *testing.T) {
	task := strings.Replace(testws.Task, "relies_on: [bel_7k3m9q]   # the one belief this rests on\n", "relies_on:\n  - bel_7k3m9q\n  # a note after the list\n", 1)
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": task})
	sync(t, w)
	revise(t, w, "bel_7k3m9q", false)
	sync(t, w) // re-pins, rewriting relies_on a second time
	if got := w.Read("task/task_9w4hzd.md"); strings.Count(got, "# a note after the list") != 1 || strings.Contains(got, ",}") {
		t.Errorf("got:\n%s", got)
	}
}

func TestReviseWarnsWhenNoCommitHoldsTheOldText(t *testing.T) {
	w := testws.New(t, map[string]string{
		"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: Reading files safely\nstatus: accepted\ncreated: 2026-11-22\n---\n\nThe design.\n",
		"task/task_9w4hzd.md":     "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nderived_from: dd_c5r1tw\ncreated: 2026-11-22\n---\n",
	})
	sync(t, w)
	p := revise(t, w, "dd_c5r1tw", false)
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0].String(), "graph/design-doc/dd_c5r1tw.md: no commit holds revision 1's text") {
		t.Errorf("warnings: %v", p.Warnings)
	}
}

func TestPinnedBeforeSyncIsResolvedBySync(t *testing.T) {
	// The form part 3, section 2 shows: a pin to a record that has no
	// revisions list yet.
	w := testws.New(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"task/task_9w4hzd.md":  "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nrelies_on:\n  - {to: bel_7k3m9q, rev: 1, fp: \"" + testws.BeliefFP + "\"}\ncreated: 2026-11-22\n---\n",
	})
	if got := problems(t, w); !strings.Contains(got, "warning: revision 1 is sealed but isn't written down yet; run `mason sync`") {
		t.Error(got)
	}
	sync(t, w)
	if got := problems(t, w); got != "" {
		t.Error(got)
	}
	if stale := w.Stale(); len(stale) != 0 {
		t.Errorf("stale: %v", stale)
	}
}

func TestKeptWordingLeavesCommentsBehind(t *testing.T) {
	w := testws.New(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(testws.Belief, "non-UTF-8 input", "non-UTF-8 input   # first seen on a Latin-1 file", 1),
		"task/task_9w4hzd.md":  testws.Task,
	})
	sync(t, w)
	revise(t, w, "bel_7k3m9q", true)
	revisions := w.Read("belief/bel_7k3m9q.md")
	revisions = revisions[strings.Index(revisions, "revisions:"):]
	if strings.Contains(revisions, "first seen") {
		t.Errorf("the comment was kept with the wording:\n%s", revisions)
	}
}

func TestSyncEditsTheFileASymlinkPointsAt(t *testing.T) {
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	real := filepath.Join(t.TempDir(), "task.md")
	os.WriteFile(real, []byte(testws.Task), 0o600)
	link := w.Path("task/task_9w4hzd.md")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	sync(t, w)
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
	w := testws.New(t, map[string]string{"belief/bel_7k3m9q.md": "\ufeff" + testws.Belief, "task/task_9w4hzd.md": testws.Task})
	sync(t, w)
	if got := w.Read("belief/bel_7k3m9q.md"); !strings.HasPrefix(got, "\ufeff---\n") || !strings.Contains(got, "revisions:") {
		t.Errorf("got %q", got)
	}
}
