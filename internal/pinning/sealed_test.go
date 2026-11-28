package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

const acceptedDecision = "---\nid: dec_9w4hzd\ntitle: Reject non-UTF-8 files\nstatus: accepted\ncreated: 2026-11-22\n---\n\nBody.\n"

// Accepting a decision seals its revision, with no task relying on it: an
// edit to its meaning fails validate, and revise records revision 2.
func TestAcceptedDecisionIsSealed(t *testing.T) {
	w := testws.New(t, map[string]string{"decision/dec_9w4hzd.md": acceptedDecision})
	sync(t, w)
	if got := problems(t, w); got != "" {
		t.Fatal(got)
	}
	w.Edit("decision/dec_9w4hzd.md", "title: Reject non-UTF-8 files", "title: Accept non-UTF-8 files")
	if got := problems(t, w); !strings.Contains(got, "the meaning changed, but revision 1 is sealed (accepted)") {
		t.Errorf("an edit in place wasn't refused:\n%s", got)
	}
	if _, err := tryRevise(t, w, "dec_9w4hzd", true); err == nil || !strings.Contains(err.Error(), "has errors") {
		t.Errorf("revise after the edit: %v", err)
	}
	w.Edit("decision/dec_9w4hzd.md", "title: Accept non-UTF-8 files", "title: Reject non-UTF-8 files")
	revise(t, w, "dec_9w4hzd", true)
	if got := w.Read("decision/dec_9w4hzd.md"); !strings.Contains(got, "rev: 2") {
		t.Errorf("no revision 2:\n%s", got)
	}
}

func TestReviseOfAProposedDecisionSaysNothingSealsIt(t *testing.T) {
	w := testws.New(t, map[string]string{"decision/dec_9w4hzd.md": strings.Replace(acceptedDecision, "accepted", "proposed", 1)})
	sync(t, w)
	if _, err := tryRevise(t, w, "dec_9w4hzd", true); err == nil || !strings.Contains(err.Error(), "nothing seals revision 1") {
		t.Errorf("got %v", err)
	}
}
