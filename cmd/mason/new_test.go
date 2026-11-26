package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	w := newWorkspace(t, nil)
	code, out, errOut := mason("new", "design-doc", "--title", "Reading files safely")
	path := strings.TrimSuffix(out, "\n")
	if code != 0 || errOut != "" || filepath.Dir(path) != filepath.Join(w.Root, "graph", "design-doc") {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "title: Reading files safely\n") {
		t.Errorf("%v\n%s", err, data)
	}
}

func TestNewMistakes(t *testing.T) {
	w := newWorkspace(t, nil)
	for want, args := range map[string][]string{
		"did you mean `belief`?":           {"new", "beleif", "--title", "X"},
		"needs --title":                    {"new", "belief"},
		"a title must be one line of text": {"new", "belief", "--title", "a\nb"},
	} {
		if code, _, errOut := mason(args...); code != 2 || !strings.Contains(errOut, want) {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
	if _, err := os.Stat(w.Path("")); err == nil {
		t.Error("a mistake still wrote to graph/")
	}
}
