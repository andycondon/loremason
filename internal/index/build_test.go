package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// ---- Build ----

// rows runs a query that returns one text column, for looking inside an
// index.
func rows(t *testing.T, w *testws.Workspace, query string, args ...any) []string {
	t.Helper()
	db, err := sql.Open("sqlite", indexPath(w))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r, err := db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []string
	for r.Next() {
		var s string
		if err := r.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestBuildWritesNodesAndContainment(t *testing.T) {
	w := workedExample(t)
	open(t, w).Close()

	for _, id := range []string{"bel_7k3m9q", "q_3fz8km", "q_3fz8km/o1"} {
		if got := rows(t, w, `SELECT id FROM nodes WHERE id = ?`, id); len(got) != 1 {
			t.Errorf("nodes has %d rows for %s, want 1", len(got), id)
		}
	}
	got := rows(t, w, `SELECT dst FROM edges WHERE src = 'q_3fz8km/o1' AND name = ?`, Containment)
	if len(got) != 1 || got[0] != "q_3fz8km" {
		t.Errorf("containment of the option = %v", got)
	}
	if got := rows(t, w, `SELECT propagates FROM edges WHERE name = ?`, Containment); len(got) == 0 || got[0] != "0" {
		t.Errorf("containment should not propagate: %v", got)
	}
	if got := rows(t, w, `SELECT count(*) FROM nodes_fts WHERE id LIKE '%/%'`); got[0] != "0" {
		t.Errorf("sub-records are in the full-text index: %v", got)
	}
}

func TestBuildStoresWhatTheGraphDecides(t *testing.T) {
	w := newWS(t, map[string]string{
		"requirement/req_5txk3a.md": requirement("req_5txk3a", "breaking"),
		"task/task_9w4hzd.md":       record("task_9w4hzd", "Task", "status: accepted", `implements: [{to: req_5txk3a, rev: 1, fp: "fp1:000000000001"}]`),
	})
	open(t, w).Close()
	got := rows(t, w, `SELECT stale || reopens || propagates FROM edges WHERE src = 'task_9w4hzd'`)
	if len(got) != 1 || got[0] != "101" {
		t.Errorf("stale, reopens, propagates = %v, want 101", got)
	}
	if got := rows(t, w, `SELECT revision || ' ' || substr(fingerprint, 1, 4) FROM nodes WHERE id = 'req_5txk3a'`); len(got) != 1 || got[0] != "2 fp1:" {
		t.Errorf("revision and fingerprint = %v", got)
	}
}

func TestBuildLeavesOutALaterFileWithTheSameID(t *testing.T) {
	w := newWS(t, map[string]string{
		"belief/bel_7k3m9q.md":   testws.Belief,
		"belief/bel_7k3m9q_2.md": testws.Belief,
	})
	x := open(t, w)
	if x.Problems() == 0 {
		t.Error("a duplicate ID should be a problem")
	}
	if got := rows(t, w, `SELECT count(*) FROM nodes WHERE id = 'bel_7k3m9q'`); got[0] != "1" {
		t.Errorf("rows for the duplicated ID: %v", got)
	}
}

func TestBuildKeepsAnUnreadableRecordOutOfTheBody(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": "---\nid: [\n---\n"})
	x := open(t, w)
	if x.Problems() == 0 {
		t.Error("an unreadable file should be a problem")
	}
}

func TestBuildFailsWhereItCannotWrite(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	g := w.Load()
	if err := Build(filepath.Join(w.Root, "missing", "index.db"), g, Meta{}); err == nil {
		t.Error("Build into a missing folder succeeded")
	}
	// A folder where the index belongs can't be replaced by a file.
	p := filepath.Join(w.Root, ".loremason", "index.db")
	if err := os.MkdirAll(p+"/x", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Build(p, g, Meta{}); err == nil {
		t.Error("Build over a folder succeeded")
	}
	if left := leftovers(t, w); len(left) != 0 {
		t.Errorf("a failed build left %v", left)
	}
}

func TestBuildReplacesAnIndexInOnePiece(t *testing.T) {
	w := newWS(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	g := w.Load()
	p := indexPath(w)
	for i := 0; i < 2; i++ {
		if err := Build(p, g, Meta{Version: itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := rows(t, w, `SELECT value FROM meta WHERE key = 'version'`); len(got) != 1 || got[0] != "1" {
		t.Errorf("version = %v", got)
	}
	if left := leftovers(t, w); len(left) != 0 {
		t.Errorf("the temporary file was left behind: %v", left)
	}
}
