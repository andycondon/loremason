package page

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/registry"
)

// ---- Plan and Apply ----

func TestPlanWritesMissingPagesAndAttributes(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	p, ds := plan(t, w)
	if got := paths(ds); got != "initiatives/.gitattributes,initiatives/alpha/README.md" {
		t.Errorf("diagnostics at %s: %v", got, ds)
	}
	if len(p.Actions) != 2 {
		t.Fatalf("actions: %+v", p.Actions)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w, AttributesPath); got != "*/README.md linguist-generated=true\n" {
		t.Errorf("attributes = %q", got)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); !strings.HasPrefix(got, "# Initiative alpha\n") {
		t.Errorf("page = %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(w.Root, "initiatives", "alpha", ".*.tmp")); len(left) != 0 {
		t.Error("a temporary file was left")
	}
}

func TestPlanCatchesAHandEditAndApplyFixesIt(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha", "purpose: Be fast.")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	write(t, w, "initiatives/alpha/README.md", strings.Replace(page, "Be fast.", "Be slow.", 1))

	p, ds := plan(t, w)
	if len(ds) != 1 || ds[0].Path != "initiatives/alpha/README.md" || ds[0].Severity == diag.Warning {
		t.Fatalf("diagnostics: %v", ds)
	}
	for _, want := range []string{"is out of date", "line 7 reads `Be slow.`", "should read `Be fast.`", "run `mason sync`"} {
		if !strings.Contains(ds[0].Msg, want) {
			t.Errorf("%q missing from %q", want, ds[0].Msg)
		}
	}
	if ds[0].Line != 7 {
		t.Errorf("line = %d, want 7", ds[0].Line)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); got != page {
		t.Errorf("the page wasn't restored:\n%s", got)
	}
	syncPages(t, w)
}

func TestPlanCatchesARecordChange(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	w.Write("task/task_t1t1t1.md", task("task_t1t1t1", "Brand new task", iniA))
	stamp(t, w, "task/task_t1t1t1.md")

	p, ds := plan(t, w)
	if len(ds) != 1 || !strings.Contains(ds[0].Msg, "is out of date") {
		t.Fatalf("diagnostics: %v", ds)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); !strings.Contains(got, "[Brand new task](../../graph/task/task_t1t1t1.md)") {
		t.Errorf("the new task isn't linked:\n%s", got)
	}
	syncPages(t, w)
}

func TestPlanLeavesAHandWrittenPageAlone(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	write(t, w, "initiatives/alpha/README.md", "# My own notes\n")
	p, ds := plan(t, w)
	if len(p.Skipped) != 1 || p.Skipped[0].Path != "initiatives/alpha/README.md" || !strings.Contains(p.Skipped[0].What, "hand-written") {
		t.Errorf("skipped: %+v", p.Skipped)
	}
	found := false
	for _, d := range ds {
		if d.Path == "initiatives/alpha/README.md" && d.Severity != diag.Warning && strings.Contains(d.Msg, "hand-written") {
			found = true
		}
	}
	if !found {
		t.Errorf("diagnostics: %v", ds)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); got != "# My own notes\n" {
		t.Errorf("the page was overwritten: %q", got)
	}
}

func TestPlanDeletesAnOrphanedPage(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	write(t, w, "initiatives/gone/README.md", Marker+"\nold\n")
	write(t, w, "initiatives/notes/README.md", "# hand-written, no initiative\n")

	p, ds := plan(t, w)
	if got := paths(ds); got != "initiatives/gone/README.md" || !strings.Contains(ds[0].Msg, "no initiative has the slug `gone`") || !strings.Contains(ds[0].Msg, "run `mason sync` to delete it") {
		t.Errorf("diagnostics: %v", ds)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if exists(w, "initiatives/gone/README.md") {
		t.Error("the orphaned page is still there")
	}
	if !exists(w, "initiatives/notes/README.md") {
		t.Error("a hand-written page with no initiative was deleted")
	}
	syncPages(t, w)
}

func TestPlanDeletesThePageOfARenamedSlug(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	edit(t, w, "initiative/"+iniA+".md", "slug: alpha", "slug: beta")
	p, _ := plan(t, w)
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if exists(w, "initiatives/alpha/README.md") || !exists(w, "initiatives/beta/README.md") {
		t.Error("the page should have moved to the new slug")
	}
	syncPages(t, w)
}

func TestPlanGivesDuplicateSlugsNoPage(t *testing.T) {
	w := workspace(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "same"),
		"initiative/" + iniB + ".md": initiative(iniB, "same"),
	})
	p, ds := plan(t, w)
	if len(p.Actions) != 0 || len(p.Skipped) != 1 || !strings.Contains(p.Skipped[0].What, iniA+", "+iniB) {
		t.Errorf("plan: %+v", p)
	}
	if len(ds) != 1 || ds[0].Path != "initiatives/same/README.md" || !strings.Contains(ds[0].Msg, "more than one initiative") {
		t.Errorf("diagnostics: %v", ds)
	}
}

func TestPlanAddsItsLineToAnAttributesFileWithoutIt(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	for _, c := range []struct{ have, want string }{
		{"*/README.md linguist-generated=false\n", "*/README.md linguist-generated=false\n*/README.md linguist-generated=true\n"},
		{"# mine\r\n*.png binary", "# mine\r\n*.png binary\n*/README.md linguist-generated=true\n"},
		{"", "*/README.md linguist-generated=true\n"},
	} {
		write(t, w, AttributesPath, c.have)
		p, ds := plan(t, w)
		if paths(ds) != AttributesPath || len(p.Actions) != 1 {
			t.Fatalf("%q: diagnostics %v, actions %+v", c.have, ds, p.Actions)
		}
		if err := p.Apply(w.Root); err != nil {
			t.Fatal(err)
		}
		if got := read(t, w, AttributesPath); got != c.want {
			t.Errorf("%q became %q, want %q", c.have, got, c.want)
		}
		syncPages(t, w)
	}
}

func TestPlanLeavesAnAttributesFileWithItsLineAlone(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	for _, have := range []string{
		"*.png binary\n*/README.md linguist-generated=true\n*.svg text\n",
		"*/README.md linguist-generated=true",
		"# mine\r\n*/README.md linguist-generated=true\r\n",
	} {
		write(t, w, AttributesPath, have)
		if p, ds := plan(t, w); len(p.Actions) != 0 || len(ds) != 0 {
			t.Errorf("%q: actions %+v, diagnostics %v", have, p.Actions, ds)
		}
	}
}

func TestPlanWithNoInitiativesWritesNothing(t *testing.T) {
	w := workspace(t, map[string]string{"task/task_t1t1t1.md": record("task_t1t1t1", "Loose", "status: accepted")})
	if p, ds := plan(t, w); len(p.Actions) != 0 || len(ds) != 0 {
		t.Errorf("plan %+v, diagnostics %v", p, ds)
	}
}

func TestPlanRecognisesAPageInAMergeConflict(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha", "purpose: Be fast.")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	conflict := strings.Replace(page, "Be fast.", "<<<<<<< ours\nBe fast.\n=======\nBe slow.\n>>>>>>> theirs", 1)
	write(t, w, "initiatives/alpha/README.md", conflict)

	p, ds := plan(t, w)
	if len(p.Skipped) != 0 || len(ds) != 1 || !strings.Contains(ds[0].Msg, "is out of date") {
		t.Fatalf("a conflicted page should be seen as generated and stale: skipped %+v, %v", p.Skipped, ds)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); got != page {
		t.Errorf("the conflict wasn't resolved:\n%s", got)
	}
}

func TestFirstDifference(t *testing.T) {
	for _, c := range []struct{ got, want, msg string }{
		{"a\nb", "a\nb\nc", "line 3 is missing"},
		{"a\nb\nc", "a\nb", "line 3 should not be there"},
		{"a\nx\n", "a\ny\n", "line 2 reads `x`, and should read `y`"},
	} {
		if _, msg := firstDifference([]byte(c.got), []byte(c.want)); !strings.HasPrefix(msg, c.msg) {
			t.Errorf("firstDifference(%q, %q) = %q", c.got, c.want, msg)
		}
	}
}

func TestPlanSeesAPageWithWindowsLineEndingsAsCurrent(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha", "purpose: Be fast.")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	write(t, w, "initiatives/alpha/README.md", strings.ReplaceAll(page, "\n", "\r\n"))
	if p, ds := plan(t, w); len(p.Actions) != 0 || len(p.Skipped) != 0 || len(ds) != 0 {
		t.Errorf("a line-ending difference is not stale: %+v, %v", p, ds)
	}
	write(t, w, "initiatives/alpha/README.md", strings.ReplaceAll(strings.Replace(page, "Be fast.", "Be slow.", 1), "\n", "\r\n"))
	p, ds := plan(t, w)
	if len(ds) != 1 || ds[0].Line != 7 || !strings.Contains(ds[0].Msg, "line 7 reads `Be slow.`") {
		t.Errorf("a real difference is still found, with no carriage return in what it says: %v", ds)
	}
	if len(p.Actions) != 1 {
		t.Errorf("actions: %+v", p.Actions)
	}
}

// ---- Which pages are generated ----

func TestPlanLeavesAPageThatOnlyQuotesTheMarkerAlone(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	notes := "# My notes\n" + strings.Repeat("a line\n", 10) + "Pages begin with:\n" + Marker + "\n"
	write(t, w, "initiatives/alpha/README.md", notes)
	p, _ := plan(t, w)
	if len(p.Skipped) != 1 || !strings.Contains(p.Skipped[0].What, "hand-written") {
		t.Errorf("a marker deep in the page makes it hand-written: %+v", p)
	}
}

func TestHasMarkerMeansAWholeLineNearTheTop(t *testing.T) {
	for _, c := range []struct {
		name string
		page string
		want bool
	}{
		{"first line", Marker + "\n", true},
		{"line 10", strings.Repeat("x\n", 9) + Marker + "\n", true},
		{"line 11", strings.Repeat("x\n", 10) + Marker + "\n", false},
		{"with a carriage return", "# T\r\n" + Marker + "\r\n", true},
		{"part of a line", "see " + Marker + "\n", false},
		{"none", "# T\n", false},
	} {
		if got := hasMarker([]byte(c.page)); got != c.want {
			t.Errorf("%s: hasMarker = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlanRecognisesAConflictAboveTheMarker(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	conflict := strings.Replace(page, "# Initiative alpha\n", "<<<<<<< ours\n# Initiative alpha\n=======\n# Initiative beta\n>>>>>>> theirs\n", 1)
	write(t, w, "initiatives/alpha/README.md", conflict)
	p, ds := plan(t, w)
	if len(p.Skipped) != 0 || len(ds) != 1 || !strings.Contains(ds[0].Msg, "is out of date") {
		t.Fatalf("a conflict above the marker should still be a generated page: %+v, %v", p.Skipped, ds)
	}
}

// ---- Pages that aren't regular files ----

func TestPlanReportsAPageThatIsNotAFile(t *testing.T) {
	w := workspace(t, map[string]string{
		"initiative/" + iniA + ".md": initiative(iniA, "alpha"),
		"initiative/" + iniB + ".md": initiative(iniB, "beta"),
	})
	if err := os.MkdirAll(filepath.Join(w.Root, "initiatives", "alpha", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, w, "initiatives/beta", "not a folder\n")
	write(t, w, "initiatives/other/x", "")
	if err := os.MkdirAll(filepath.Join(w.Root, "initiatives", "other", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	p, ds := plan(t, w)
	for _, page := range []string{"initiatives/alpha/README.md", "initiatives/beta/README.md"} {
		found := false
		for _, d := range ds {
			if d.Path == page && d.Severity != diag.Warning && strings.Contains(d.Msg, "not a regular file") {
				found = true
			}
		}
		if !found {
			t.Errorf("no diagnostic for %s: %v", page, ds)
		}
		skipped := false
		for _, a := range p.Skipped {
			if a.Path == page {
				skipped = true
			}
		}
		if !skipped {
			t.Errorf("%s isn't skipped: %+v", page, p.Skipped)
		}
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
}

// ---- Broken records ----

func TestPlanLeavesPagesAloneWhileARecordCannotBeLoaded(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	w.Write("initiative/"+iniA+".md", "---\nid: [\n---\n")
	stamp(t, w, "initiative/"+iniA+".md")

	x, err := index.Open(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if x.Problems() == 0 {
		t.Fatal("the broken record should be a problem")
	}
	p, ds, err := Plan(w.Root, x)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Actions) != 0 || len(ds) != 0 {
		t.Errorf("a page whose initiative can't be read is not an orphan: %+v, %v", p.Actions, ds)
	}
	if got := read(t, w, "initiatives/alpha/README.md"); got != page {
		t.Errorf("the page changed:\n%s", got)
	}
}

// ---- Apply ----

func TestApplyIsSafeWhenSeveralRunAtOnce(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	p, _ := plan(t, w)
	for round := 0; round < 30; round++ {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := p.Apply(w.Root); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(w.Root, "initiatives", "*", ".*.tmp")); len(left) != 0 {
		t.Errorf("temporary files were left: %v", left)
	}
	syncPages(t, w)
}

func TestApplyLeavesNoTemporaryFileWhenItFails(t *testing.T) {
	dir := t.TempDir()
	// A folder where the file belongs can't be replaced by a file.
	if err := os.MkdirAll(filepath.Join(dir, "page.md", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "page.md"), []byte("x")); err == nil {
		t.Fatal("writeFile over a folder succeeded")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) != 0 {
		t.Errorf("a failed write left %v", left)
	}
}
