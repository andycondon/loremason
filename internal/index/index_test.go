package index

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// ---- Open ----

func TestOpenBuildsThenReuses(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	x := open(t, w)
	if !x.Rebuilt() {
		t.Error("the first open should build the index")
	}
	if _, err := os.Stat(indexPath(w)); err != nil {
		t.Errorf("no index file: %v", err)
	}
	x.Close()

	x = open(t, w)
	if x.Rebuilt() {
		t.Error("nothing changed, so the second open should reuse the index")
	}
	if _, err := os.Stat(indexPath(w) + ".tmp"); err == nil {
		t.Error("the temporary file was left behind")
	}
}

func TestOpenFollowsTheFiles(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()

	reopen := func(change string) *Index {
		t.Helper()
		x := open(t, w)
		if !x.Rebuilt() {
			t.Errorf("%s should make the next open rebuild", change)
		}
		return x
	}

	put(t, w, "belief/bel_c9d4se.md", strings.NewReplacer("bel_7k3m9q", "bel_c9d4se", "validate and encodings", "guessing").Replace(testws.Belief))
	x := reopen("an added record")
	if got, err := x.Show("bel_c9d4se"); err != nil || got.Title != "guessing" {
		t.Errorf("added record: %+v, %v", got, err)
	}
	x.Close()

	// The same size and the same name: only the modification time differs.
	edit(t, w, "belief/bel_c9d4se.md", "guessing", "gussing ")
	x = reopen("an edited record")
	if got, err := x.Show("bel_c9d4se"); err != nil || got.Title != "gussing" {
		t.Errorf("edited record: %+v, %v", got, err)
	}
	x.Close()

	if err := os.Remove(w.Path("belief/bel_c9d4se.md")); err != nil {
		t.Fatal(err)
	}
	x = reopen("a deleted record")
	if _, err := x.Show("bel_c9d4se"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted record: %v", err)
	}
	x.Close()

	if x = open(t, w); x.Rebuilt() {
		t.Error("nothing changed after the last rebuild")
	}
}

func TestOpenRebuildsForAnotherVersionOrMethod(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()

	if err := os.WriteFile(filepath.Join(w.Root, ".loremason", "config.yaml"), []byte("version: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	x := open(t, w)
	if !x.Rebuilt() {
		t.Error("another loremason version should rebuild")
	}
	x.Close()

	err := buildWith(t, w, fingerprint.Method+1)
	if err != nil {
		t.Fatal(err)
	}
	if x = open(t, w); !x.Rebuilt() {
		t.Error("another fingerprint method should rebuild")
	}
}

// buildWith builds the workspace's index as if another fingerprint method made
// it.
func buildWith(t *testing.T, w *testws.Workspace, method int) error {
	t.Helper()
	sig, err := Signature(w.Root)
	if err != nil {
		return err
	}
	return Build(indexPath(w), w.Load(), Meta{Version: "other", FingerprintMethod: method, Signature: sig})
}

func TestOpenRebuildsACorruptIndex(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()
	if err := os.WriteFile(indexPath(w), []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	x := open(t, w)
	if !x.Rebuilt() {
		t.Error("an unreadable index should be rebuilt")
	}
	if _, err := x.Show("bel_7k3m9q"); err != nil {
		t.Error(err)
	}
}

func TestOpenWritesGitignoreOnce(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()
	p := filepath.Join(w.Root, ".loremason", ".gitignore")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"index.db", "index.db.tmp"} {
		if !strings.Contains(string(data), want+"\n") {
			t.Errorf(".gitignore lacks %s:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "config.yaml") {
		t.Errorf(".gitignore hides the config:\n%s", data)
	}
	if err := os.WriteFile(p, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	open(t, w).Close()
	if data, _ = os.ReadFile(p); string(data) != "mine\n" {
		t.Errorf("an existing .gitignore was overwritten:\n%s", data)
	}
}

func TestOpenWithoutWorkspace(t *testing.T) {
	if _, err := Open(t.TempDir(), registry.Load()); err == nil {
		t.Error("a folder with no workspace opened")
	}
}

func TestOpenWithoutGraphFolder(t *testing.T) {
	w := newWS(t, nil)
	x := open(t, w)
	if got, err := x.Find("anything"); err != nil || len(got) != 0 {
		t.Errorf("Find = %v, %v", got, err)
	}
}

func TestOpenWithoutFTS5(t *testing.T) {
	old := probeFTS5
	t.Cleanup(func() { probeFTS5 = old })
	probeFTS5 = func(db *sql.DB) error { return errors.New("no such module: fts5") }

	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	_, err := Open(w.Root, registry.Load())
	if err == nil {
		t.Fatal("Open succeeded without FTS5")
	}
	for _, want := range []string{"full-text search (FTS5) is missing", "rebuild mason", "no such module: fts5"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if _, serr := os.Stat(indexPath(w)); serr == nil {
		t.Error("an index was left in place")
	}
}

func TestProbeFTS5IsSatisfiedByThisBuild(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

// ---- Signature ----

func TestSignatureChangesWithTheFiles(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	sig := func() string {
		t.Helper()
		s, err := Signature(w.Root)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	first := sig()
	if first != sig() {
		t.Error("the signature changed with the files")
	}
	stamp(t, w, "belief/bel_7k3m9q.md")
	touched := sig()
	if touched == first {
		t.Error("a new modification time didn't change the signature")
	}
	put(t, w, "belief/other.md", "x")
	if sig() == touched {
		t.Error("an added file didn't change the signature")
	}
}

func TestSignatureOfNoGraph(t *testing.T) {
	if s, err := Signature(t.TempDir()); err != nil || s != "" {
		t.Errorf("Signature = %q, %v", s, err)
	}
}

// ---- Problems ----

func TestProblemsCountsErrorsNotWarnings(t *testing.T) {
	w := newWS(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"task/task_9w4hzd.md":  record("task_9w4hzd", "Task", "status: accepted", "relies_on: [bel_zzzzzz]"),
		"belief/broken.md":     "---\nid: [\n---\n",
	})
	x := open(t, w)
	if x.Problems() < 2 {
		t.Errorf("Problems = %d, want the unreadable file and the missing target", x.Problems())
	}
	want := x.Problems()
	x.Close()
	if x = open(t, w); x.Rebuilt() || x.Problems() != want {
		t.Errorf("after reopening: rebuilt %v, Problems = %d, want %d", x.Rebuilt(), x.Problems(), want)
	}
	got, err := x.Show("task_9w4hzd")
	if err != nil || got.Title != "Task" {
		t.Errorf("a graph with errors still answers: %+v, %v", got, err)
	}
}

func TestProblemsOfACleanWorkspace(t *testing.T) {
	x := open(t, workedExample(t))
	if x.Problems() != 0 {
		t.Errorf("Problems = %d, want 0", x.Problems())
	}
}
