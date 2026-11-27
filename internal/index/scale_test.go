package index

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// ---- workspaces of 3,000 records ----

const scaleRecords = 3000

var (
	scaleOnce sync.Once
	scaleRoot string
	scaleErr  error
)

// scaleWorkspace returns the root of a generated workspace of 3,000 records,
// made once for all the benchmarks and tests that use it. It lives in a
// folder the testing package removes when the run ends.
func scaleWorkspace(tb testing.TB) string {
	tb.Helper()
	scaleOnce.Do(func() {
		scaleRoot = scaleDir()
		scaleErr = testws.Generate(scaleRoot, scaleRecords, 1)
	})
	if scaleErr != nil {
		tb.Fatal(scaleErr)
	}
	return scaleRoot
}

// scaleFacts finds a record that many others rely on and a word in its
// title, so each query has something to do.
func scaleFacts(tb testing.TB, x *Index) (id, word string) {
	tb.Helper()
	err := x.db.QueryRow(`SELECT e.dst, n.title FROM edges e JOIN nodes n ON n.id = e.dst
		WHERE e.propagates = 1 GROUP BY e.dst ORDER BY count(*) DESC, e.dst LIMIT 1`).Scan(&id, &word)
	if err != nil {
		tb.Fatal(err)
	}
	return id, strings.Fields(word)[0]
}

func openScale(tb testing.TB) *Index {
	tb.Helper()
	x, err := Open(scaleWorkspace(tb), registry.Load())
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { x.Close() })
	return x
}

func BenchmarkOpenCurrent(b *testing.B) {
	scaleWorkspace(b)
	openScale(b).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x, err := Open(scaleRoot, registry.Load())
		if err != nil {
			b.Fatal(err)
		}
		x.Close()
	}
}

func BenchmarkBuild(b *testing.B) {
	root := scaleWorkspace(b)
	g := loadGraph(b, root)
	path := root + "/.loremason/bench.db"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Build(path, g, Meta{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind(b *testing.B) {
	x := openScale(b)
	_, word := scaleFacts(b, x)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := x.Find(word); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShow(b *testing.B) {
	x := openScale(b)
	id, _ := scaleFacts(b, x)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := x.Show(id); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDependents(b *testing.B) {
	x := openScale(b)
	id, _ := scaleFacts(b, x)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := x.Dependents(id); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStale(b *testing.B) {
	x := openScale(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := x.Stale(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReopened(b *testing.B) {
	x := openScale(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := x.Reopened(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestScale builds the index of 3,000 records, then runs each query, and
// wants each to take under a second. That is far above what they need, so
// it only fails when something has gone badly wrong. -short skips it.
func TestScale(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 3,000-record workspace")
	}
	root := scaleWorkspace(t)
	removeAll(t, root)

	within := func(what string, f func() error) {
		t.Helper()
		start := time.Now()
		if err := f(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if d := time.Since(start); d > slowdown*time.Second {
			t.Errorf("%s took %v, want under %dx1s", what, d, slowdown)
		} else {
			t.Logf("%s took %v", what, d)
		}
	}

	var x *Index
	within("Open with a rebuild", func() (err error) {
		x, err = Open(root, registry.Load())
		return err
	})
	t.Cleanup(func() { x.Close() })
	if !x.Rebuilt() {
		t.Fatal("the index should have been built")
	}
	id, word := scaleFacts(t, x)
	within("Find", func() error {
		got, err := x.Find(word)
		if err == nil && len(got) == 0 {
			t.Errorf("Find(%q) found nothing", word)
		}
		return err
	})
	within("Show", func() error { _, err := x.Show(id); return err })
	within("Dependents", func() error {
		got, err := x.Dependents(id)
		if err == nil && len(got) == 0 {
			t.Errorf("Dependents(%s) found nothing", id)
		}
		return err
	})
	within("Stale", func() error { _, err := x.Stale(); return err })
	within("Reopened", func() error { _, err := x.Reopened(); return err })
}
