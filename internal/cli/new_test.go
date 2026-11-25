package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
)

func emptyWorkspace(t *testing.T) string {
	t.Helper()
	useVersion(t, "test")
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".loremason"), 0o755)
	os.WriteFile(filepath.Join(root, ".loremason", "config.yaml"), []byte("version: test\n"), 0o644)
	t.Setenv("LOREMASON_WORKSPACE", root)
	return root
}

// Titles that YAML could misread, plus one long enough to tempt it to wrap.
var titles = []string{
	"X", "42", "true", "2026-11-22", "a: b", "# not a comment", `"quoted" and 'single'`,
	"[list]", "{title} {today}", "日本語", "- dash", "@at", strings.Repeat("long title ", 30) + "end",
}

func TestNewRecordsAreValid(t *testing.T) {
	root := emptyWorkspace(t)
	for _, name := range registry.Load().Names() {
		typ := registry.Load().Types[name]
		for _, title := range titles {
			code, out, errOut := run("new", name, "--title", title)
			path := strings.TrimSuffix(out, "\n")
			if code != 0 {
				t.Fatalf("%s %q: exit %d, %s", name, title, code, errOut)
			}
			if rel, _ := filepath.Rel(root, path); !regexp.MustCompile(`^graph/` + typ.Folder() + `/` + typ.Prefix + `_[0-9a-z]{6}\.md$`).MatchString(filepath.ToSlash(rel)) {
				t.Errorf("%s was written to %s", name, rel)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			r, errs := record.Parse(data)
			if len(errs) > 0 || r.Fields["title"] != title {
				t.Fatalf("%s %q: %v\n%s", name, title, errs, data)
			}
		}
	}
	// Titles with no letters share a slug, so duplicates may be warned about.
	if code, out, _ := run("validate"); code != 0 {
		t.Errorf("new records aren't valid:\n%s", out)
	}
}

func TestNewTakesFlagsFirstAndKebabCase(t *testing.T) {
	emptyWorkspace(t)
	if code, out, errOut := run("new", "--title", "X", "design-doc"); code != 0 || !strings.Contains(out, "design-doc") {
		t.Fatalf("exit %d: %s%s", code, out, errOut)
	}
}

func TestNewMistakes(t *testing.T) {
	root := emptyWorkspace(t)
	for want, args := range map[string][]string{
		"did you mean `belief`?":  {"new", "beleif", "--title", "X"},
		"the types are activity,": {"new", "xyzzyplugh", "--title", "X"},
		"needs --title":           {"new", "belief"},
		"one line of text":        {"new", "belief", "--title", "a\nb"},
		"takes one record type":   {"new", "belief", "--title", "two", "words"},
		"unknown command `nwe`":   {"nwe"},
	} {
		if code, _, errOut := run(args...); code != 2 || !strings.Contains(errOut, want) {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "graph")); err == nil {
		t.Error("a mistake still wrote to graph/")
	}
}
