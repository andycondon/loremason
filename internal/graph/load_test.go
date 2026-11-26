package graph

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ---- What Load reads ----

func TestLoadWorkedExample(t *testing.T) {
	g, ds := loadRoot(t, "../../testdata/worked-example")
	if len(ds) > 0 || len(g.Files) != 26 {
		t.Fatalf("%d files:\n%s", len(g.Files), lines(ds))
	}
	// Every link resolves to a type its edge allows.
	for _, e := range g.Edges {
		if e.Target == nil || !e.Def.Allows(e.Target.Kind) {
			t.Errorf("%s %s -> %s doesn't resolve to an allowed type", e.From.ID, e.Name, e.To)
		}
	}
}

func TestLoadWithoutAGraphFolder(t *testing.T) {
	g, ds := loadRoot(t, t.TempDir())
	if len(g.Files) != 0 || len(ds) != 0 {
		t.Errorf("%d files:\n%s", len(g.Files), lines(ds))
	}
}

func TestLoadRecordsWhatEachFileIs(t *testing.T) {
	text := file("bel_7k3m9q", "claim: a claim")
	g, _ := loadFiles(t, map[string]string{"question/bel_7k3m9q.md": text})
	f := g.Files[0]
	if f.Path != "graph/question/bel_7k3m9q.md" || f.Folder.Name != "question" || f.Type.Name != "belief" {
		t.Errorf("path %s, folder %s, type %s", f.Path, f.Folder.Name, f.Type.Name)
	}
	if f.Sum != sha256.Sum256([]byte(text)) {
		t.Error("Sum isn't the file's SHA-256")
	}
	if n := f.Node; n.ID != "bel_7k3m9q" || n.Kind != &f.Type.Kind || n.File != f || n.Fields.Text("claim") != "a claim" {
		t.Errorf("node: %+v", n)
	}
}

func TestLoadSkipsHiddenFiles(t *testing.T) {
	g, ds := loadFiles(t, map[string]string{
		".cache/x.md":               "anything",
		"belief/.bel_7k3m9q.md.swp": "anything",
		"belief/bel_7k3m9q.md":      file("bel_7k3m9q"),
	})
	if len(g.Files) != 1 || len(ds) != 0 {
		t.Errorf("%d files:\n%s", len(g.Files), lines(ds))
	}
}

func TestLoadFollowsSymlinks(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	write(t, filepath.Join(elsewhere, "kept.md"), file("bel_7k3m9q"))
	os.MkdirAll(filepath.Join(root, Dir, "belief"), 0o755)
	if err := os.Symlink(filepath.Join(elsewhere, "kept.md"), filepath.Join(root, Dir, "belief", "bel_7k3m9q.md")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	os.Symlink(elsewhere, filepath.Join(root, Dir, "belief", "bel_folder0.md"))
	g, ds := loadRoot(t, root)
	if len(g.Files) != 1 || g.Files[0].Node.ID != "bel_7k3m9q" || lines(ds) != "graph/belief/bel_folder0.md:1: error: only record files (<id>.md) belong here" {
		t.Errorf("%d files:\n%s", len(g.Files), lines(ds))
	}
}

// ---- What Load keeps out, and says why ----

func TestLoadReportsWhatItKeepsOut(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"a file outside a type folder",
			map[string]string{"bel_7k3m9q.md": file("bel_7k3m9q")},
			"graph/bel_7k3m9q.md:1: error: records are kept in a folder named after their type, such as graph/belief/"},
		{"a folder no type has",
			map[string]string{"belif/bel_7k3m9q.md": file("bel_7k3m9q")},
			"graph/belif:1: error: no record type has the folder `belif`; did you mean `belief`?"},
		{"a folder inside a type folder",
			map[string]string{"belief/old/bel_7k3m9q.md": file("bel_7k3m9q")},
			"graph/belief/old:1: error: records aren't kept in folders inside a type's folder"},
		{"a file that isn't a record",
			map[string]string{"belief/notes.txt": "notes"},
			"graph/belief/notes.txt:1: error: only record files (<id>.md) belong here"},
		{"a second record with an ID",
			map[string]string{"belief/bel_7k3m9q.md": file("bel_7k3m9q"), "question/bel_7k3m9q.md": file("bel_7k3m9q")},
			"graph/question/bel_7k3m9q.md:1: error: `bel_7k3m9q` is also the ID of graph/belief/bel_7k3m9q.md"},
		{"an alias that is another record's ID",
			map[string]string{"belief/bel_7k3m9q.md": file("bel_7k3m9q", "aliases: [q_3fz8km]"), "question/q_3fz8km.md": file("q_3fz8km")},
			"graph/belief/bel_7k3m9q.md:4: error: the alias `q_3fz8km` is the ID of a record in graph/question/q_3fz8km.md"},
		{"an alias already taken",
			map[string]string{"belief/bel_2wq8dn.md": file("bel_2wq8dn", "aliases: [B-1]"), "belief/bel_7k3m9q.md": file("bel_7k3m9q", "aliases: [B-1]")},
			"graph/belief/bel_7k3m9q.md:4: error: the alias `B-1` is already used by graph/belief/bel_2wq8dn.md"},
		{"two sub-records with one id",
			map[string]string{"belief/bel_7k3m9q.md": file("bel_7k3m9q", "support:", "  - {id: s1}", "  - {id: s1}")},
			"graph/belief/bel_7k3m9q.md:6: error: two entries have the id `s1`"},
		{"a file that doesn't parse",
			map[string]string{"belief/bel_7k3m9q.md": "no header\n"},
			"graph/belief/bel_7k3m9q.md:1: error: the file must start with a YAML header between two `---` lines"},
	} {
		if _, ds := loadFiles(t, c.files); lines(ds) != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, lines(ds), c.want)
		}
	}
}

func TestLoadReportsAFileItCantRead(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	root := t.TempDir()
	p := filepath.Join(root, Dir, "belief", "bel_7k3m9q.md")
	write(t, p, file("bel_7k3m9q"))
	os.Chmod(p, 0)
	t.Cleanup(func() { os.Chmod(p, 0o644) })
	g, ds := loadRoot(t, root)
	// The message doesn't repeat the path, and the record still has a node.
	if lines(ds) != "graph/belief/bel_7k3m9q.md:1: error: can't read: permission denied" || g.Node("bel_7k3m9q") == nil {
		t.Errorf("%s", lines(ds))
	}
}

// A file that can't be parsed still has a node under its filename's ID, so
// links to it resolve and aren't reported as links to nothing.
func TestLoadKeepsANodeForAFileThatDoesntParse(t *testing.T) {
	g, _ := loadFiles(t, map[string]string{
		"belief/bel_7k3m9q.md": "---\nid: bel_7k3m9q\ntitle: [unclosed\n---\n",
		"task/task_9w4hzd.md":  file("task_9w4hzd", "relies_on: [bel_7k3m9q]"),
	})
	n := g.Node("bel_7k3m9q")
	if n == nil || n.File.Record != nil || edge(t, g, "task_9w4hzd", "relies_on").Target != n {
		t.Fatalf("node: %+v", n)
	}
	if !n.File.Rejected(2) || n.File.Rejected(1) {
		t.Error("the line with the YAML error is the one rejected")
	}
}

// ---- Sub-records, aliases and edges ----

func TestLoadSubrecords(t *testing.T) {
	g := load(t, file("bel_7k3m9q", "support:", "  - {id: s1, evidence: ev_p4n2xa}", "  - {evidence: ev_p4n2xa}", "  - {id: s2}"))
	bel := g.Node("bel_7k3m9q")
	// The entry with no id is the schema check's to report; it isn't a node.
	if !slices.Equal(ids(bel.Children), []string{"bel_7k3m9q/s1", "bel_7k3m9q/s2"}) {
		t.Fatalf("children: %v", ids(bel.Children))
	}
	s1 := g.Node("bel_7k3m9q/s1")
	if s1 != bel.Children[0] || s1.Parent != bel || s1.Local != "s1" || s1.Kind.Name != "support_link" || s1.File != bel.File || s1.Fields.Text("evidence") != "ev_p4n2xa" {
		t.Errorf("s1: %+v", s1)
	}
}

func TestLoadAliases(t *testing.T) {
	g := load(t, file("bel_7k3m9q", "aliases: [B-1, 42, \"\", [x]]"))
	// Values that aren't names are skipped: the schema check reports them.
	if !slices.Equal(g.Names(), []string{"B-1", "bel_7k3m9q"}) {
		t.Errorf("names: %v", g.Names())
	}
}

func TestLoadEdges(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "claim: a claim", "support:", "  - {id: s1, evidence: ev_p4n2xa}"),
		file("ev_p4n2xa"),
		file("ev_m3q8vb"),
		file("task_9w4hzd",
			"relies_on:",
			"  - bel_7k3m9q",
			"  - {to: bel_7k3m9q, rev: 1, fp: \"fp1:536221e3130c\", via: ev_m3q8vb}",
			"  - bel_nope00",
			"  - 42",
			"  - not a ref",
			"  - {role: lead}",
			"implements: req_5txk3a@r2",
		),
	)
	var got []string
	for _, e := range g.Edges {
		target := "nothing"
		if e.Target != nil {
			target = e.Target.ID
		}
		got = append(got, fmt.Sprintf("%s:%d ", e.At.Path, e.At.Line)+e.At.Field.String()+": "+e.From.ID+" "+e.Name+" -> "+e.To.String()+" = "+target)
	}
	// Values of the wrong shape (42, not a ref, a map with no to) are
	// skipped: the schema check reports them.
	want := []string{
		"graph/belief/bel_7k3m9q.md:6 support[0].evidence: bel_7k3m9q/s1 evidence -> ev_p4n2xa = ev_p4n2xa",
		"graph/task/task_9w4hzd.md:11 implements: task_9w4hzd implements -> req_5txk3a@r2 = nothing",
		"graph/task/task_9w4hzd.md:5 relies_on[0]: task_9w4hzd relies_on -> bel_7k3m9q = bel_7k3m9q",
		"graph/task/task_9w4hzd.md:6 relies_on[1].to: task_9w4hzd relies_on -> bel_7k3m9q = bel_7k3m9q",
		"graph/task/task_9w4hzd.md:6 relies_on[1].via: task_9w4hzd via -> ev_m3q8vb = ev_m3q8vb",
		"graph/task/task_9w4hzd.md:7 relies_on[2]: task_9w4hzd relies_on -> bel_nope00 = nothing",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if bel := g.Node("bel_7k3m9q"); len(bel.Incoming) != 2 {
		t.Errorf("incoming: %d", len(bel.Incoming))
	}
	if via := edge(t, g, "task_9w4hzd", "via"); via.Pin != nil || !via.Def.Allows(g.Node("ev_m3q8vb").Kind) || via.Def.Allows(g.Node("bel_7k3m9q").Kind) {
		t.Errorf("a pin's via is an unpinned edge to evidence: %+v", via)
	}
}
