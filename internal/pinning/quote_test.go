package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

const (
	quotedFinding = "---\nid: fnd_k3m9qx\ntitle: Caching helps\ncreated: 2026-11-22\n---\n\nFirst thoughts.\n"
	quote         = "---\nid: ev_p4n2xa\ntitle: A quote\nobserved_at: 2026-11-22T10:00:00Z\nfrom: [fnd_k3m9qx]\ncreated: 2026-11-22\n---\n\nThe words.\n"
)

// Sync pins the quote to the finding as it stands, then the finding is
// rewritten. Nothing is held to the old wording.
func TestQuotedFindingStaysFreeToEdit(t *testing.T) {
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": quotedFinding, "evidence/ev_p4n2xa.md": quote})
	sync(t, w)
	pinned := w.Read("evidence/ev_p4n2xa.md")
	if !strings.Contains(pinned, "{to: fnd_k3m9qx, rev: 1, fp: \"fp1:") {
		t.Fatalf("sync didn't pin the quote:\n%s", pinned)
	}

	w.Edit("finding/fnd_k3m9qx.md", "First thoughts.", "Second thoughts, much changed.")
	// Editing is allowed. It only warns that the quote no longer matches.
	got := problems(t, w)
	if errs, _ := checked(t, w).Diagnostics.Count(); errs != 0 || !strings.Contains(got, "fnd_k3m9qx has changed since it was quoted at r1; run `mason revise fnd_k3m9qx`") {
		t.Errorf("editing a quoted finding should only warn about the quote:\n%s", got)
	}
	p := sync(t, w)
	if got := actions(p); !strings.Contains(got, "refresh the fingerprint of revision 1, which nothing seals") {
		t.Errorf("sync didn't refresh the finding's fingerprint:\n%s", got)
	}
	if n := w.Node("fnd_k3m9qx"); n.Current().FP != n.Fingerprint() {
		t.Error("the finding's recorded fingerprint isn't its current one")
	}
	if got := w.Read("evidence/ev_p4n2xa.md"); got != pinned {
		t.Errorf("sync touched the quote:\n%s", got)
	}
	if s := w.Stale(); len(s) != 0 {
		t.Errorf("stale: %v", s)
	}
}

// A quote's pin stays at the revision it quoted, across a non-breaking
// revision that would move a sealing pin forward.
func TestSyncLeavesAQuoteOnItsRevision(t *testing.T) {
	finding := "---\nid: fnd_k3m9qx\ntitle: Caching helps\nrevisions:\n  - {rev: 1, date: 2026-11-22, fp: \"fp1:000000000001\"}\n  - {rev: 2, date: 2026-11-23, fp: \"fp1:000000000002\", change: non_breaking}\ncreated: 2026-11-22\n---\n\nFirst thoughts.\n"
	quote := strings.Replace(quote, "[fnd_k3m9qx]", "\n  - {to: fnd_k3m9qx, rev: 1, fp: \"fp1:000000000001\"}", 1)
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": finding, "evidence/ev_p4n2xa.md": quote})
	sync(t, w)
	// Sync adds the evidence's own fingerprint, and nothing else.
	got := w.Read("evidence/ev_p4n2xa.md")
	if !strings.Contains(got, "{to: fnd_k3m9qx, rev: 1, fp: \"fp1:000000000001\"}") || strings.Contains(got, "rev: 2") {
		t.Errorf("the quote moved:\n%s", got)
	}
	if s := w.Stale(); len(s) != 0 {
		t.Errorf("a quote of an older revision is stale: %v", s)
	}
}

// A quote that names an older revision with @r1 is pinned to it, not moved
// forward as a link that propagates would be.
func TestSyncPinsAQuoteToTheRevisionItNames(t *testing.T) {
	finding := "---\nid: fnd_k3m9qx\ntitle: Caching helps\nrevisions:\n  - {rev: 1, date: 2026-11-22, fp: \"fp1:000000000001\"}\n  - {rev: 2, date: 2026-11-23, fp: \"fp1:000000000002\", change: non_breaking}\ncreated: 2026-11-22\n---\n\nFirst thoughts.\n"
	named := strings.Replace(quote, "[fnd_k3m9qx]", "[fnd_k3m9qx@r1]", 1)
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": finding, "evidence/ev_p4n2xa.md": named})
	p := sync(t, w)
	if got := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(got, "{to: fnd_k3m9qx, rev: 1, fp: \"fp1:000000000001\"}") {
		t.Errorf("the quote isn't pinned to r1:\n%s", got)
	}
	if got := actions(p); strings.Contains(got, "since every revision") {
		t.Errorf("the pin moved forward:\n%s", got)
	}
}

// An evidence link to any of the four document types is pinned at the
// document's current revision.
func TestSyncPinsEvidenceToEachDocumentType(t *testing.T) {
	docs := map[string]string{
		"finding/fnd_k3m9qx.md":   "---\nid: fnd_k3m9qx\ntitle: F\ncreated: 2026-11-22\n---\n\nText.\n",
		"prd/prd_k3m9qx.md":       "---\nid: prd_k3m9qx\ntitle: P\nstatus: proposed\ncreated: 2026-11-22\n---\n\nText.\n",
		"design-doc/dd_k3m9qx.md": "---\nid: dd_k3m9qx\ntitle: D\nstatus: proposed\ncreated: 2026-11-22\n---\n\nText.\n",
		"plan/plan_k3m9qx.md":     "---\nid: plan_k3m9qx\ntitle: L\nstatus: proposed\ncreated: 2026-11-22\n---\n\nText.\n",
	}
	for path, text := range docs {
		id := strings.TrimSuffix(path[strings.Index(path, "/")+1:], ".md")
		w := testws.New(t, map[string]string{path: text, "evidence/ev_p4n2xa.md": strings.Replace(quote, "[fnd_k3m9qx]", "["+id+"]", 1)})
		sync(t, w)
		if got := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(got, "{to: "+id+", rev: 1, fp: \"fp1:") {
			t.Errorf("%s: the quote isn't pinned at revision 1:\n%s", id, got)
		}
		if got := problems(t, w); got != "" {
			t.Errorf("%s: %s", id, got)
		}
	}
}

// A pin that names an older revision is compared with that revision's
// recorded fingerprint, not the finding's current text.
func TestQuoteOfAnOlderRevisionComparesWithThatRevision(t *testing.T) {
	finding := "---\nid: fnd_k3m9qx\ntitle: Caching helps\nrevisions:\n  - {rev: 1, date: 2026-11-22, fp: \"fp1:000000000001\"}\n  - {rev: 2, date: 2026-11-23, fp: \"fp1:000000000002\", change: non_breaking}\ncreated: 2026-11-22\n---\n\nFirst thoughts.\n"
	pin := func(fp string) string {
		return strings.Replace(quote, "[fnd_k3m9qx]", "\n  - {to: fnd_k3m9qx, rev: 1, fp: \""+fp+"\"}", 1)
	}
	const drift = "the text quoted from fnd_k3m9qx@r1 has changed since it was quoted"

	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": finding, "evidence/ev_p4n2xa.md": pin("fp1:000000000001")})
	sync(t, w)
	if got := problems(t, w); strings.Contains(got, drift) {
		t.Errorf("a quote that matches revision 1 warned, though the finding is at revision 2:\n%s", got)
	}

	w = testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": finding, "evidence/ev_p4n2xa.md": pin("fp1:0000000000aa")})
	if got := problems(t, w); !strings.Contains(got, drift) {
		t.Errorf("a quote that differs from revision 1's fingerprint didn't warn:\n%s", got)
	}
}

// Editing a quoted finding in place warns; restoring the text clears it.
func TestQuoteDriftClearsWhenTheTextIsRestored(t *testing.T) {
	const drift = "fnd_k3m9qx has changed since it was quoted at r1; run `mason revise fnd_k3m9qx`"
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": quotedFinding, "evidence/ev_p4n2xa.md": quote})
	sync(t, w)
	if got := problems(t, w); got != "" {
		t.Fatalf("a fresh quote drew diagnostics:\n%s", got)
	}
	w.Edit("finding/fnd_k3m9qx.md", "First thoughts.", "Second thoughts.")
	if got := problems(t, w); !strings.Contains(got, drift) || !strings.Contains(got, "warning") {
		t.Errorf("no drift warning:\n%s", got)
	}
	w.Edit("finding/fnd_k3m9qx.md", "Second thoughts.", "First thoughts.")
	if got := problems(t, w); got != "" {
		t.Errorf("restoring the text didn't clear the warning:\n%s", got)
	}
}

const fixedEvidence = "---\nid: ev_p4n2xa\ntitle: Seen\nobserved_at: 2026-11-22T10:00:00Z\ncreated: 2026-11-22\n---\n\nThe words.\n"

// Sync records the evidence's fingerprint, then validate is clean; dry-run
// says the same step without writing; a later edit is an error; evidence
// with no fingerprint warns.
func TestEvidenceKeepsItsOwnFingerprint(t *testing.T) {
	w := testws.New(t, map[string]string{"evidence/ev_p4n2xa.md": fixedEvidence})
	if got := problems(t, w); !strings.Contains(got, "warning") || !strings.Contains(got, "the fingerprint of ev_p4n2xa isn't written down yet; run `mason sync`") {
		t.Errorf("evidence without fp didn't warn:\n%s", got)
	}
	p := Sync(checked(t, w).Graph, nil, today)
	if got := actions(p); got != "graph/evidence/ev_p4n2xa.md: record the fingerprint of ev_p4n2xa" {
		t.Errorf("dry-run says:\n%s", got)
	}
	if got := w.Read("evidence/ev_p4n2xa.md"); got != fixedEvidence {
		t.Errorf("planning wrote the file:\n%s", got)
	}
	sync(t, w)
	if got := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(got, "fp: \""+w.Node("ev_p4n2xa").Fingerprint().String()+"\"") {
		t.Errorf("sync didn't write the fingerprint:\n%s", got)
	}
	if got := problems(t, w); got != "" {
		t.Errorf("validate after sync:\n%s", got)
	}
	if p := sync(t, w); !p.Empty() {
		t.Errorf("a second sync: %s", actions(p))
	}

	w.Edit("evidence/ev_p4n2xa.md", "The words.", "Other words.")
	got := problems(t, w)
	if !strings.Contains(got, "error") || !strings.Contains(got, "ev_p4n2xa never changes") || !strings.Contains(got, "write new evidence that undermines this one") {
		t.Errorf("editing evidence didn't fail validate:\n%s", got)
	}
	if p := sync(t, w); !p.Empty() {
		t.Errorf("sync rewrote an edited evidence's fingerprint: %s", actions(p))
	}
}
