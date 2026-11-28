package check

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// Each document type, written with one revision whose recorded fingerprint
// doesn't match its meaning, so a check that holds a pin to it would say so.
var documents = map[string]string{
	"finding/fnd_k3m9qx.md":   "id: fnd_k3m9qx\ntitle: A finding",
	"design-doc/dd_c5r1tw.md": "id: dd_c5r1tw\ntitle: A design doc\nstatus: proposed",
	"prd/prd_8h2vnc.md":       "id: prd_8h2vnc\ntitle: A PRD\nstatus: proposed",
	"plan/plan_4q7zbd.md":     "id: plan_4q7zbd\ntitle: A plan\nstatus: proposed",
}

func document(header string) string {
	return "---\n" + header + "\nrevisions:\n  - {rev: 1, date: 2026-11-22, fp: \"fp1:000000000000\"}\ncreated: 2026-11-22\n---\n\nSome words.\n"
}

func evidenceFrom(id, to string) string {
	return "---\nid: " + id + "\ntitle: A quote\nobserved_at: 2026-11-22T10:00:00Z\nfrom:\n  - {to: " + to + ", rev: 1, fp: \"fp1:000000000000\"}\ncreated: 2026-11-22\n---\n\nThe words.\n"
}

func TestEvidenceQuotesEachDocumentType(t *testing.T) {
	records := map[string]string{}
	n := 0
	for path, header := range documents {
		records[path] = document(header)
		id := strings.TrimSuffix(path[strings.Index(path, "/")+1:], ".md")
		records["evidence/ev_p4n2x"+string(rune('a'+n))+".md"] = evidenceFrom("ev_p4n2x"+string(rune('a'+n)), id)
		n++
	}
	w := testws.New(t, records)
	if _, got := run(t, w.Root); got != "" {
		t.Errorf("a pinned quote of a document is rejected:\n%s", got)
	}
}

func TestEvidenceCantQuoteADecision(t *testing.T) {
	w := testws.New(t, map[string]string{
		"decision/dec_6m4xkp.md": "---\nid: dec_6m4xkp\ntitle: A decision\nstatus: proposed\ncreated: 2026-11-22\n---\n",
		"evidence/ev_p4n2xa.md":  "---\nid: ev_p4n2xa\ntitle: A quote\nobserved_at: 2026-11-22T10:00:00Z\nfrom: [dec_6m4xkp]\ncreated: 2026-11-22\n---\n\nThe words.\n",
	})
	_, got := run(t, w.Root)
	for _, want := range []string{"error", "actor", "artifact", "document"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}

func TestIdeationIsFree(t *testing.T) {
	w := testws.New(t, map[string]string{
		"finding/fnd_k3m9qx.md": "---\nid: fnd_k3m9qx\ntitle: Half an idea\ncreated: 2026-11-22\n---\n\nMaybe caching helps.\n",
	})
	if _, got := run(t, w.Root); got != "" {
		t.Errorf("a finding with a title and a body drew diagnostics:\n%s", got)
	}
}

// A quoted finding has a revision that no longer matches its wording, and
// it's fine. A design doc pinned by a task is not.
func TestQuotedFindingIsFreeButPinnedDesignDocIsNot(t *testing.T) {
	w := testws.New(t, map[string]string{
		"finding/fnd_k3m9qx.md":   document(documents["finding/fnd_k3m9qx.md"]),
		"evidence/ev_p4n2xa.md":   evidenceFrom("ev_p4n2xa", "fnd_k3m9qx"),
		"design-doc/dd_c5r1tw.md": document(documents["design-doc/dd_c5r1tw.md"]),
		"task/task_9w4hzd.md":     "---\nid: task_9w4hzd\ntitle: A task\nstatus: accepted\nderived_from:\n  - {to: dd_c5r1tw, rev: 1, fp: \"fp1:000000000000\"}\ncreated: 2026-11-22\n---\n",
	})
	_, got := run(t, w.Root)
	if strings.Contains(got, "fnd_k3m9qx") || strings.Contains(got, "ev_p4n2xa") {
		t.Errorf("the quoted finding or its quote was reported:\n%s", got)
	}
	if !strings.Contains(got, "the meaning changed, but revision 1 is pinned by task_9w4hzd") {
		t.Errorf("the pinned design doc wasn't held to its revision:\n%s", got)
	}
}
