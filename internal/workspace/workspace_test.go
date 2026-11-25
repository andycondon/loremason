package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newWorkspace(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".loremason"), 0o755)
	os.MkdirAll(filepath.Join(root, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(root, ".loremason", "config.yaml"), []byte(config), 0o644)
	return root
}

func TestFindWalksUp(t *testing.T) {
	root := newWorkspace(t, "version: 1.2.3\n")
	w, err := Find(filepath.Join(root, "a", "b"), "")
	if err != nil || w.Root != root || w.Config.Version != "1.2.3" {
		t.Fatalf("got %+v, %v", w, err)
	}
}

func TestOverrideWins(t *testing.T) {
	here, there := newWorkspace(t, "version: 1\n"), newWorkspace(t, "version: 2\n")
	w, err := Find(here, there)
	if err != nil || w.Root != there {
		t.Fatalf("got %+v, %v", w, err)
	}
}

func TestErrors(t *testing.T) {
	plain := t.TempDir()
	for name, c := range map[string]struct{ dir, override, want string }{
		"no workspace":    {plain, "", "no workspace here"},
		"bad override":    {newWorkspace(t, "version: 1\n"), plain, Env},
		"unknown setting": {newWorkspace(t, "version: 1\nversoin: 2\n"), "", "versoin"},
		"empty config":    {newWorkspace(t, ""), "", "`version` is missing"},
	} {
		if _, err := Find(c.dir, c.override); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
