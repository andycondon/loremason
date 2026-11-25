package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the expected output of fixtures")

func useVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

// Each folder in testdata/validate is a workspace; want.txt holds what
// `mason validate` should print there, and its exit code.
func TestValidateFixtures(t *testing.T) {
	useVersion(t, "test")
	dirs, _ := filepath.Glob("testdata/validate/*")
	if len(dirs) < 10 {
		t.Fatalf("only %d fixtures found", len(dirs))
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Setenv("LOREMASON_WORKSPACE", dir)
			code, out, errOut := run("validate")
			if errOut != "" {
				t.Fatalf("stderr: %s", errOut)
			}
			got := fmt.Sprintf("%sexit %d\n", out, code)
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
	useVersion(t, "test")
	t.Setenv("LOREMASON_WORKSPACE", "../../testdata/worked-example")
	code, out, errOut := run("validate")
	if code != 0 || errOut != "" || !strings.HasSuffix(out, " records: 0 errors, 0 warnings\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
}

func TestValidateFindsTheWorkspaceFromInside(t *testing.T) {
	useVersion(t, "test")
	t.Setenv("LOREMASON_WORKSPACE", "")
	t.Chdir("../../testdata/worked-example/graph/belief")
	if code, out, _ := run("validate"); code != 0 || !strings.Contains(out, "0 errors") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestVersionMismatchWarns(t *testing.T) {
	useVersion(t, "other")
	t.Setenv("LOREMASON_WORKSPACE", "../../testdata/worked-example")
	code, _, errOut := run("validate")
	if code != 0 || !strings.Contains(errOut, "this is mason other, but the workspace uses test") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestNoWorkspace(t *testing.T) {
	t.Setenv("LOREMASON_WORKSPACE", "")
	t.Chdir(t.TempDir())
	if code, _, errOut := run("validate"); code != 1 || !strings.Contains(errOut, "no workspace here") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadableFileIsReportedAndTheRestChecked(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	useVersion(t, "test")
	root := t.TempDir()
	copyDir(t, "../../testdata/worked-example", root)
	locked := filepath.Join(root, "graph", "belief", "bel_7k3m9q.md")
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	t.Setenv("LOREMASON_WORKSPACE", root)
	code, out, _ := run("validate")
	if code != 1 || !strings.Contains(out, "graph/belief/bel_7k3m9q.md:1: error: can't read: permission denied\n") || !strings.Contains(out, "records: 1 error") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestSymlinksAreFollowed(t *testing.T) {
	useVersion(t, "test")
	elsewhere, root := t.TempDir(), t.TempDir()
	copyDir(t, "../../testdata/worked-example", elsewhere)
	os.Mkdir(filepath.Join(root, ".loremason"), 0o755)
	os.WriteFile(filepath.Join(root, ".loremason", "config.yaml"), []byte("version: test\n"), 0o644)
	if err := os.Symlink(filepath.Join(elsewhere, "graph"), filepath.Join(root, "graph")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	// A record that is itself a link, under its own ID.
	os.Rename(filepath.Join(elsewhere, "graph", "belief", "bel_c9d4se.md"), filepath.Join(elsewhere, "kept.md"))
	os.Symlink(filepath.Join(elsewhere, "kept.md"), filepath.Join(elsewhere, "graph", "belief", "bel_c9d4se.md"))
	t.Setenv("LOREMASON_WORKSPACE", root)
	code, out, _ := run("validate")
	if code != 0 || out != "26 records: 0 errors, 0 warnings\n" {
		t.Fatalf("exit %d\n%s", code, out)
	}
}
