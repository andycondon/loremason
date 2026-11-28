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
	if got := problems(t, w); got != "" {
		t.Errorf("editing a quoted finding drew diagnostics:\n%s", got)
	}
	p := sync(t, w)
	if got := actions(p); !strings.Contains(got, "refresh the fingerprint of revision 1, which no propagating link pins") {
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
	if got := w.Read("evidence/ev_p4n2xa.md"); got != quote {
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
