package check

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

// Titles that YAML could misread, plus one long enough to tempt it to wrap.
var titles = []string{
	"X", "日本語 one", "日本語 two", "!!!", "42", "true", "2026-11-22", "a: b", "# not a comment", `"quoted" and 'single'`,
	"[list]", "{title} {today}", "日本語", "- dash", "@at", strings.Repeat("long title ", 30) + "end",
}

// A record of every type, made by graph.Create, passes every check.
func TestNewRecordsPassEveryCheck(t *testing.T) {
	w := testws.New(t, nil)
	reg := registry.Load()
	for _, name := range reg.Names() {
		typ := reg.Types[name]
		for _, title := range titles {
			rel, err := graph.Create(w.Root, typ, title, time.Now(), graph.CreateOptions{})
			if err != nil {
				t.Fatalf("%s %q: %v", name, title, err)
			}
			if !regexp.MustCompile(`^graph/` + typ.Folder() + `/` + typ.Prefix + `_[0-9a-z]{6}\.md$`).MatchString(rel) {
				t.Errorf("%s was written to %s", name, rel)
			}
			r, errs := record.Parse([]byte(w.Read(strings.TrimPrefix(rel, "graph/"))))
			if len(errs) > 0 || r.Data["title"] != title {
				t.Fatalf("%s %q: %v", name, title, errs)
			}
		}
	}
	if _, got := run(t, w.Root); got != "" {
		t.Errorf("new records aren't valid:\n%s", got)
	}
}
