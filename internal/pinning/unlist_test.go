package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

func activityRecord(id, session string) string {
	return "---\nid: " + id + "\ntitle: Session\nsession: " + session + "\nstarted_at: 2026-11-22\ncreated: 2026-11-22\n---\n"
}

func evidenceGeneratedBy(id, generated string) string {
	return "---\nid: " + id + "\ntitle: Seen\nobserved_at: 2026-11-22T10:00:00Z\nfrom: []\n" + generated + "created: 2026-11-22\n---\n\nThe words.\n"
}

// A list of one link, or an empty list, where a single link goes is
// rewritten by sync to the link, or dropped.
func TestSyncRewritesAListWhereOneLinkGoes(t *testing.T) {
	w := testws.New(t, map[string]string{
		"activity/actv_3h7npc.md": activityRecord("actv_3h7npc", "a"),
		"evidence/ev_p4n2xa.md":   evidenceGeneratedBy("ev_p4n2xa", "generated_by: [actv_3h7npc]\n"),
		"evidence/ev_w2q9kd.md":   evidenceGeneratedBy("ev_w2q9kd", "generated_by: []\n"),
	})
	p := sync(t, w)
	got := actions(p)
	for _, want := range []string{
		"ev_p4n2xa.md: rewrite `generated_by` from a one-item list to a single value",
		"ev_w2q9kd.md: drop the empty `generated_by` list",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no action %q in:\n%s", want, got)
		}
	}
	if one := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(one, "\ngenerated_by: actv_3h7npc\n") || strings.Contains(one, "generated_by: [") {
		t.Errorf("one-item list:\n%s", one)
	}
	if none := w.Read("evidence/ev_w2q9kd.md"); strings.Contains(none, "generated_by") {
		t.Errorf("empty list:\n%s", none)
	}
}

// A longer list is the person's to resolve, so sync leaves it.
func TestSyncLeavesALongerListAlone(t *testing.T) {
	src := evidenceGeneratedBy("ev_p4n2xa", "generated_by: [actv_3h7npc, actv_9z1fgw]\n")
	w := testws.New(t, map[string]string{
		"activity/actv_3h7npc.md": activityRecord("actv_3h7npc", "a"),
		"activity/actv_9z1fgw.md": activityRecord("actv_9z1fgw", "b"),
		"evidence/ev_p4n2xa.md":   src,
	})
	p := sync(t, w)
	if got := actions(p); strings.Contains(got, "generated_by") {
		t.Errorf("actions:\n%s", got)
	}
	if got := w.Read("evidence/ev_p4n2xa.md"); !strings.Contains(got, "generated_by: [actv_3h7npc, actv_9z1fgw]") {
		t.Errorf("file:\n%s", got)
	}
}
