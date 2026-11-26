package graph

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/registry"
)

func TestCreateKeepsToTheTitleRule(t *testing.T) {
	belief, _ := registry.Load().Find("belief")
	root := t.TempDir()
	for _, bad := range []string{"", "   ", "a\nb", "tab\there", "bad \xff byte"} {
		if _, err := Create(root, belief, bad, time.Now()); !errors.Is(err, ErrTitle) {
			t.Errorf("Create(%q): %v", bad, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, Dir)); err == nil {
		t.Error("a bad title still wrote to graph/")
	}
	rel, err := Create(root, belief, "  spaced  ", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	g, ds, _ := Load(root, registry.Load())
	if len(ds) > 0 || len(g.Files) != 1 || g.Files[0].Path != rel || g.Files[0].Node.Fields.Text("title") != "spaced" {
		t.Errorf("got %v, %+v", ds, g.Files)
	}
}
