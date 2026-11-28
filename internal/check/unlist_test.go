package check

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

func activityRecord(id, session string) string {
	return "---\nid: " + id + "\ntitle: Session\nsession: " + session + "\nstarted_at: 2026-11-22\ncreated: 2026-11-22\n---\n"
}

func evidenceGeneratedBy(generated string) string {
	return "---\nid: ev_p4n2xa\ntitle: Seen\nobserved_at: 2026-11-22T10:00:00Z\nfrom: []\ngenerated_by: " + generated + "\ncreated: 2026-11-22\n---\n\nThe words.\n"
}

// A list of one link where a single link goes is an error that sync
// repairs, so it doesn't keep sync away from the file.
func TestListWhereOneLinkGoesIsFixable(t *testing.T) {
	for _, generated := range []string{"[actv_3h7npc]", "[]"} {
		w := testws.New(t, map[string]string{
			"activity/actv_3h7npc.md": activityRecord("actv_3h7npc", "a"),
			"evidence/ev_p4n2xa.md":   evidenceGeneratedBy(generated),
		})
		res, out := run(t, w.Root)
		if !strings.Contains(out, "`generated_by` is a list") || !strings.Contains(out, "mason sync") {
			t.Errorf("%s: diagnostics:\n%s", generated, out)
		}
		if len(res.Broken()) != 0 {
			t.Errorf("%s: file is broken: %v", generated, res.Broken())
		}
	}
}

func TestLongerListWhereOneLinkGoesIsAnError(t *testing.T) {
	w := testws.New(t, map[string]string{
		"activity/actv_3h7npc.md": activityRecord("actv_3h7npc", "a"),
		"activity/actv_9z1fgw.md": activityRecord("actv_9z1fgw", "b"),
		"evidence/ev_p4n2xa.md":   evidenceGeneratedBy("[actv_3h7npc, actv_9z1fgw]"),
	})
	res, out := run(t, w.Root)
	if !strings.Contains(out, "error: `generated_by` is a list") || strings.Contains(out, "rewrites it") {
		t.Errorf("diagnostics:\n%s", out)
	}
	if len(res.Broken()) != 1 {
		t.Errorf("broken: %v", res.Broken())
	}
}

// A one-item list whose item isn't a plain ID is the person's to resolve:
// the check calls it an error, not one that sync repairs.
func TestListOfAMapIsNotFixable(t *testing.T) {
	w := testws.New(t, map[string]string{
		"activity/actv_3h7npc.md": activityRecord("actv_3h7npc", "a"),
		"evidence/ev_p4n2xa.md":   evidenceGeneratedBy("[{to: actv_3h7npc}]"),
	})
	res, out := run(t, w.Root)
	if strings.Contains(out, "rewrites it") {
		t.Errorf("diagnostics:\n%s", out)
	}
	if len(res.Broken()) != 1 {
		t.Errorf("broken: %v", res.Broken())
	}
}
