package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/registry"
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
	got := rows(t, w, `SELECT concerns || ' ' || propagates FROM edges WHERE src = 'task_9w4hzd'`)
	if len(got) != 1 || got[0] != "1 1" {
		t.Errorf("concerns, propagates = %v, want the Stale flag (1) and 1", got)
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

func TestBuildStoresProgressSettledLinksMembersAndInitiatives(t *testing.T) {
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha", "purpose: Be fast.", "goals:", "  - First", "  - Second",
			"involves:", "  - {to: who_k1m2n3, role: decides}"),
		"actor/who_k1m2n3.md":    record("who_k1m2n3", "Zed", "kind: person"),
		"task/task_t1t1t1.md":    task("task_t1t1t1", "Working", iniA, "progress: in_progress"),
		"question/q_c1c1c1.md":   question("q_c1c1c1", iniA),
		"question/q_c2c2c2.md":   question("q_c2c2c2", iniA),
		"decision/dec_d2d2d2.md": decision("dec_d2d2d2", iniA, "accepted", "q_c2c2c2"),
		"decision/dec_d5d5d5.md": decision("dec_d5d5d5", iniA, "proposed", "q_c1c1c1"),
	})
	valid(t, w)
	open(t, w).Close()

	one := func(want, query string, args ...any) {
		t.Helper()
		if got := rows(t, w, query, args...); len(got) != 1 || got[0] != want {
			t.Errorf("%s %v = %v, want [%s]", query, args, got, want)
		}
	}
	one("in_progress", `SELECT progress FROM nodes WHERE id = 'task_t1t1t1'`)
	one("0", `SELECT open FROM nodes WHERE id = 'q_c2c2c2'`)
	one("1", `SELECT open FROM nodes WHERE id = 'q_c1c1c1'`)
	one("1", `SELECT settles FROM edges WHERE src = 'dec_d2d2d2' AND dst = 'q_c2c2c2'`)
	one("0", `SELECT settles FROM edges WHERE src = 'dec_d5d5d5' AND dst = 'q_c1c1c1'`)
	one("1", `SELECT pending FROM edges WHERE src = 'dec_d5d5d5' AND dst = 'q_c1c1c1'`)
	one("0", `SELECT pending FROM edges WHERE src = 'dec_d2d2d2' AND dst = 'q_c2c2c2'`)
	one("decides", `SELECT role FROM edges WHERE src = ? AND dst = 'who_k1m2n3'`, iniA)
	if got := rows(t, w, `SELECT id FROM members WHERE initiative = ? ORDER BY id`, iniA); strings.Join(got, " ") != "dec_d2d2d2 dec_d5d5d5 q_c1c1c1 q_c2c2c2 task_t1t1t1" {
		t.Errorf("members = %v", got)
	}
	one("alpha|Be fast.|[\"First\",\"Second\"]", `SELECT slug || '|' || purpose || '|' || goals FROM initiatives WHERE id = ?`, iniA)
}

// ---- Build outside the workspace ----

func TestOpenBuildsOutsideAWorkspaceItCannotWriteTo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	w := newWS(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
	})
	dir := filepath.Join(w.Root, ".loremason")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	x, err := Open(w.Root, registry.Load())
	if err != nil {
		t.Fatalf("Open in a read-only workspace: %v", err)
	}
	if !x.Rebuilt() {
		t.Error("with no index, the open should build one")
	}
	if got, err := x.Initiatives(); err != nil || len(got) != 1 || got[0].Slug != "alpha" {
		t.Errorf("Initiatives = %+v, %v", got, err)
	}
	if _, err := os.Stat(indexPath(w)); err == nil {
		t.Error("an index was written into the read-only folder")
	}
	temp := x.temp
	if temp == "" {
		t.Fatal("the index should be in a temporary file")
	}
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temp); err == nil {
		t.Errorf("Close left %s behind", temp)
	}
}
