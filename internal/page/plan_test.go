package page

import (
	"strings"
	"testing"
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
	if exists(w, "initiatives/alpha/.README.md.tmp") {
		t.Error("a temporary file was left")
	}
}

func TestPlanCatchesAHandEditAndApplyFixesIt(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha", "purpose: Be fast.")})
	syncPages(t, w)
	page := read(t, w, "initiatives/alpha/README.md")
	write(t, w, "initiatives/alpha/README.md", strings.Replace(page, "Be fast.", "Be slow.", 1))

	p, ds := plan(t, w)
	if len(ds) != 1 || ds[0].Path != "initiatives/alpha/README.md" || ds[0].Warning {
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
		if d.Path == "initiatives/alpha/README.md" && !d.Warning && strings.Contains(d.Msg, "hand-written") {
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
	if got := paths(ds); got != "initiatives/gone/README.md" || !strings.Contains(ds[0].Msg, "run `mason sync` to delete it") {
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

func TestPlanRewritesTheAttributesFile(t *testing.T) {
	w := workspace(t, map[string]string{"initiative/" + iniA + ".md": initiative(iniA, "alpha")})
	syncPages(t, w)
	write(t, w, AttributesPath, "*/README.md linguist-generated=false\n")
	p, ds := plan(t, w)
	if paths(ds) != AttributesPath || len(p.Actions) != 1 {
		t.Fatalf("diagnostics %v, actions %+v", ds, p.Actions)
	}
	if err := p.Apply(w.Root); err != nil {
		t.Fatal(err)
	}
	syncPages(t, w)
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
