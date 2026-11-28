package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// Regression tests for what a review of the revision rules found.

// Revising an accepted decision opens a revision nothing seals yet, so the
// edit it was opened for is allowed. Once sync records it, the new revision
// is sealed like any other.
func TestReviseOfAnAcceptedDecisionLetsTheEditThrough(t *testing.T) {
	w := testws.New(t, map[string]string{"decision/dec_9w4hzd.md": acceptedDecision})
	sync(t, w)
	revise(t, w, "dec_9w4hzd", true)
	w.Edit("decision/dec_9w4hzd.md", "title: Reject non-UTF-8 files", "title: Accept non-UTF-8 files")
	if got := problems(t, w); strings.Contains(got, "error") {
		t.Errorf("the edit after revise was refused before sync:\n%s", got)
	}
	sync(t, w)
	if got := problems(t, w); got != "" {
		t.Errorf("validate after sync:\n%s", got)
	}
	w.Edit("decision/dec_9w4hzd.md", "title: Accept non-UTF-8 files", "title: Something else")
	if got := problems(t, w); !strings.Contains(got, "the meaning changed, but revision 2 is sealed (accepted)") {
		t.Errorf("a second edit wasn't refused:\n%s", got)
	}
}

// An accepted record whose revision has no fingerprint yet is reported as
// waiting for sync, not as sealed.
func TestAcceptedRevisionWithNoFingerprintWarnsToSync(t *testing.T) {
	w := testws.New(t, map[string]string{"decision/dec_9w4hzd.md": acceptedDecision})
	sync(t, w)
	revise(t, w, "dec_9w4hzd", false)
	got := problems(t, w)
	if !strings.Contains(got, "revision 2 isn't written down yet") || !strings.Contains(got, "run `mason sync`") || strings.Contains(got, "error") {
		t.Errorf("got:\n%s", got)
	}
}

// Evidence still holding the template body isn't fixed yet: sync says so and
// leaves its fingerprint unwritten, so it can still be written.
func TestSyncLeavesEvidenceWithTheTemplateBodyAlone(t *testing.T) {
	const template = "The verbatim quote or observation. Never edited once written."
	ev := "---\nid: ev_p4n2xa\ntitle: Seen\nobserved_at: 2026-11-22T10:00:00Z\nfrom: []\ncreated: 2026-11-22\n---\n\n" + template + "\n"
	w := testws.New(t, map[string]string{"evidence/ev_p4n2xa.md": ev})
	p := sync(t, w)
	if got := actions(p); strings.Contains(got, "fingerprint") {
		t.Errorf("sync recorded a fingerprint:\n%s", got)
	}
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0].String(), "ev_p4n2xa still has the template body; write it, then run mason sync") {
		t.Errorf("warnings: %v", p.Warnings)
	}
	if got := problems(t, w); !strings.Contains(got, "the fingerprint of ev_p4n2xa isn't written down yet") {
		t.Errorf("validate:\n%s", got)
	}
	// Written, even with different spacing in the template text kept as is.
	w.Edit("evidence/ev_p4n2xa.md", template, "The tool printed 42.")
	p = sync(t, w)
	if got := actions(p); !strings.Contains(got, "record the fingerprint of ev_p4n2xa") || len(p.Warnings) != 0 {
		t.Errorf("after writing it: %v, warnings %v", got, p.Warnings)
	}
	if got := problems(t, w); got != "" {
		t.Errorf("validate:\n%s", got)
	}
}

// A quote doesn't seal, but it lets the quoted finding be revised: the quote
// keeps naming revision 1, whose fingerprint is kept, so the drift warning
// clears.
func TestReviseOfAQuotedFindingKeepsTheQuoteAtItsRevision(t *testing.T) {
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": quotedFinding, "evidence/ev_p4n2xa.md": quote})
	sync(t, w)
	revise(t, w, "fnd_k3m9qx", true)
	w.Edit("finding/fnd_k3m9qx.md", "First thoughts.", "Second thoughts, much changed.")
	sync(t, w)
	if got := problems(t, w); got != "" {
		t.Errorf("validate after revise, edit and sync:\n%s", got)
	}
	if got := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(got, "rev: 1") {
		t.Errorf("the quote no longer names revision 1:\n%s", got)
	}
	if n := w.Node("fnd_k3m9qx"); len(n.Revisions) != 2 || n.Revision(1).FP.IsZero() {
		t.Errorf("revisions: %+v", n.Revisions)
	}
}

// Revising something nothing seals or quotes still refuses.
func TestReviseOfAnUnquotedFindingStillRefuses(t *testing.T) {
	w := testws.New(t, map[string]string{"finding/fnd_k3m9qx.md": quotedFinding})
	sync(t, w)
	if _, err := tryRevise(t, w, "fnd_k3m9qx", true); err == nil || !strings.Contains(err.Error(), "nothing seals revision 1") {
		t.Errorf("got %v", err)
	}
}
