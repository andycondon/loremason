package index

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
	"github.com/andycondon/loremason/schema"
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
	if left := leftovers(t, w); len(left) != 0 {
		t.Errorf("the temporary file was left behind: %v", left)
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

func TestOpenRebuildsForAnOlderIndexSchemaOrChangedTypes(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()
	current := func() Meta {
		t.Helper()
		sig, err := Signature(w.Root)
		if err != nil {
			t.Fatal(err)
		}
		return Meta{Version: testws.Version, Schema: SchemaVersion, Types: typesHash(), FingerprintMethod: fingerprint.Method, Signature: sig}
	}
	if x := open(t, w); x.Rebuilt() {
		t.Fatal("the test's idea of a current index is wrong: Open rebuilt it")
	}
	for name, change := range map[string]func(*Meta){
		"an older index schema": func(m *Meta) { m.Schema-- },
		"changed record types":  func(m *Meta) { m.Types = "another" },
	} {
		m := current()
		change(&m)
		if err := Build(indexPath(w), w.Load(), m); err != nil {
			t.Fatal(err)
		}
		x := open(t, w)
		if !x.Rebuilt() {
			t.Errorf("%s should rebuild", name)
		}
		x.Close()
		if x = open(t, w); x.Rebuilt() {
			t.Errorf("after the rebuild for %s the index should be current", name)
		}
	}
}

// withSchemaFiles makes typesHash read files instead of the embedded schema
// until the test ends.
func withSchemaFiles(t *testing.T, files fs.FS) {
	t.Helper()
	old := schemaFiles
	schemaFiles = files
	t.Cleanup(func() { schemaFiles = old })
}

// copySchema copies the embedded schema files, so a test can change one.
func copySchema(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	err := fs.WalkDir(schema.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(schema.FS, p)
		files[p] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTypesHashCoversEverySchemaFile(t *testing.T) {
	files := copySchema(t)
	withSchemaFiles(t, files)
	base := typesHash()
	if typesHash() != base {
		t.Fatal("the hash should be stable")
	}
	var jsons int
	for name := range files {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		jsons++
		was := files[name].Data
		// The same length, so only the bytes differ.
		files[name] = &fstest.MapFile{Data: append(append([]byte{}, was[:len(was)-1]...), was[len(was)-1]^1)}
		if typesHash() == base {
			t.Errorf("a changed byte in %s doesn't change the hash", name)
		}
		files[name] = &fstest.MapFile{Data: was}
	}
	if jsons == 0 {
		t.Fatal("no JSON schema files found")
	}
	files["added.json"] = &fstest.MapFile{Data: []byte("{}")}
	if typesHash() == base {
		t.Error("an added file doesn't change the hash")
	}
}

func TestOpenRebuildsWhenASchemaFileChanges(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()
	if x := open(t, w); x.Rebuilt() {
		t.Fatal("nothing changed")
	}

	files := copySchema(t)
	data := files["task.json"].Data
	files["task.json"] = &fstest.MapFile{Data: append(append([]byte{}, data...), '\n')}
	withSchemaFiles(t, files)
	if x := open(t, w); !x.Rebuilt() {
		t.Error("a changed schema file should make the next open rebuild")
	}
}

// ---- FromCheck ----

func TestFromCheckBuildsOnceThenReusesACurrentIndex(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	sig, err := Signature(w.Root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	x, err := FromCheck(w.Root, res, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !x.Rebuilt() {
		t.Error("the first FromCheck should build the index")
	}
	if got, err := x.Show("bel_7k3m9q"); err != nil || got.Title != "validate and encodings" {
		t.Errorf("built from the result: %+v, %v", got, err)
	}
	x.Close()
	if _, err := os.Stat(filepath.Join(w.Root, ".loremason", ".gitignore")); err != nil {
		t.Errorf("FromCheck should write the .gitignore as Open does: %v", err)
	}

	// The index is current, so the result isn't read: nil would panic if it were.
	x, err = FromCheck(w.Root, nil, sig)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if x.Rebuilt() {
		t.Error("the second FromCheck should reuse the index")
	}
	// Open sees the same index, so the two share what they build.
	if y := open(t, w); y.Rebuilt() {
		t.Error("Open should reuse the index FromCheck built")
	}
}

func TestFromCheckWithAnOldSignatureMakesTheNextOpenRebuild(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	sig, err := Signature(w.Root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	// A file edited while the command ran.
	edit(t, w, "belief/bel_7k3m9q.md", "validate and encodings", "validate and encoding")
	x, err := FromCheck(w.Root, res, sig)
	if err != nil {
		t.Fatal(err)
	}
	x.Close()
	if y := open(t, w); !y.Rebuilt() {
		t.Error("the signature taken before the load is old, so the next open should rebuild")
	}
}

func TestOpenInParallelOnAStaleIndex(t *testing.T) {
	w := newWS(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"task/task_9w4hzd.md":  record("task_9w4hzd", "Task", "status: accepted", "relies_on: [bel_7k3m9q]"),
	})
	open(t, w).Close()
	put(t, w, "belief/bel_c9d4se.md", strings.NewReplacer("bel_7k3m9q", "bel_c9d4se", "validate and encodings", "guessing").Replace(testws.Belief))

	const n = 8
	got := make([]answers, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			x, err := Open(w.Root, registry.Load())
			if err != nil {
				errs[i] = err
				return
			}
			defer x.Close()
			if got[i], err = answersOf(x, "bel_7k3m9q"); err != nil {
				errs[i] = err
			}
		}()
	}
	close(start)
	wg.Wait()
	for i := range got {
		if errs[i] != nil {
			t.Fatalf("open %d: %v", i, errs[i])
		}
		if !reflect.DeepEqual(got[i], got[0]) {
			t.Errorf("open %d answered differently:\n%+v\nvs\n%+v", i, got[i], got[0])
		}
	}
	if len(got[0].Dependents) != 1 || got[0].Show == nil {
		t.Errorf("answers look empty: %+v", got[0])
	}
	if _, err := open(t, w).Show("bel_c9d4se"); err != nil {
		t.Errorf("the index lacks the added record: %v", err)
	}
	if left := leftovers(t, w); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
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

// ignored reports whether a .gitignore of simple patterns covers name.
func ignored(gitignore, name string) bool {
	for _, pat := range strings.Fields(gitignore) {
		if ok, _ := path.Match(pat, name); ok {
			return true
		}
	}
	return false
}

func TestOpenWritesGitignoreOnce(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	open(t, w).Close()
	p := filepath.Join(w.Root, ".loremason", ".gitignore")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Every file a build or a crashed one can leave is covered, and the
	// config isn't.
	for _, name := range []string{"index.db", "index.db.123456.tmp", "index.db-journal", "index.db-wal"} {
		if !ignored(string(data), name) {
			t.Errorf(".gitignore doesn't cover %s:\n%s", name, data)
		}
	}
	if ignored(string(data), "config.yaml") {
		t.Errorf(".gitignore covers config.yaml:\n%s", data)
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

func TestOpenReportsABusyProbeAsItIs(t *testing.T) {
	old := probeFTS5
	t.Cleanup(func() { probeFTS5 = old })
	probeFTS5 = func(db *sql.DB) error { return errors.New("database is locked") }

	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	_, err := Open(w.Root, registry.Load())
	if err == nil || !strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "FTS5") {
		t.Errorf("Open = %v; want the busy error, not a missing-FTS5 one", err)
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

func TestConnectOpensARelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	db, err := connect(filepath.Join(".", "sub", "..", "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (a)`); err != nil {
		t.Fatalf("a relative path with .. should open the file: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "x.db")); err != nil || info.Size() == 0 {
		t.Errorf("the table wasn't written to ./x.db: %v", err)
	}
}
