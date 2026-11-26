package check

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/registry"
)

var update = flag.Bool("update", false, "rewrite each fixture's want.txt with what the check reports")

// run checks a workspace and returns its diagnostics, one per line.
func run(t *testing.T, root string) (*Result, string) {
	t.Helper()
	res, err := Run(root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	if res.Records == 0 {
		t.Fatalf("%s holds no records", root)
	}
	var out strings.Builder
	for _, d := range res.Diagnostics {
		out.WriteString(d.String() + "\n")
	}
	return res, out.String()
}

// Each folder in testdata/check, at the top of the repo, is a workspace for
// one rule. Its want.txt holds every diagnostic the check reports there.
func TestFixtures(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/check/*")
	if len(dirs) < 10 {
		t.Fatalf("only %d fixtures found", len(dirs))
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			_, got := run(t, dir)
			path := filepath.Join(dir, "want.txt")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestWorkedExampleIsValid(t *testing.T) {
	if _, got := run(t, "../../testdata/worked-example"); got != "" {
		t.Error(got)
	}
}

func TestUnreadableFileIsReportedAndTheRestChecked(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../testdata/worked-example")); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "graph", "belief", "bel_7k3m9q.md")
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if _, got := run(t, root); got != "graph/belief/bel_7k3m9q.md:1: error: can't read: permission denied\n" {
		t.Errorf("got:\n%s", got)
	}
}

func TestSymlinksAreFollowed(t *testing.T) {
	elsewhere, root := t.TempDir(), t.TempDir()
	if err := os.CopyFS(elsewhere, os.DirFS("../../testdata/worked-example")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(elsewhere, "graph"), filepath.Join(root, "graph")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	// A record that is itself a link, under its own ID.
	os.Rename(filepath.Join(elsewhere, "graph", "belief", "bel_c9d4se.md"), filepath.Join(elsewhere, "kept.md"))
	os.Symlink(filepath.Join(elsewhere, "kept.md"), filepath.Join(elsewhere, "graph", "belief", "bel_c9d4se.md"))
	if res, got := run(t, root); res.Records != 26 || got != "" {
		t.Errorf("%d records:\n%s", res.Records, got)
	}
}
