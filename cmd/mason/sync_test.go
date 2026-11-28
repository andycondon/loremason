package main

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

func TestSync(t *testing.T) {
	newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": testws.Task})
	code, out, errOut := mason("sync")
	if code != 0 || errOut != "" || !strings.Contains(out, "graph/task/task_9w4hzd.md: pin `relies_on[0]` to bel_7k3m9q@r1\n") {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
	if code, out, _ := mason("sync"); code != 0 || out != "nothing to sync\n" {
		t.Errorf("second sync: exit %d\n%s", code, out)
	}
}

func TestSyncDryRun(t *testing.T) {
	w := newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": testws.Task})
	code, out, _ := mason("sync", "--dry-run")
	if code != 0 || !strings.Contains(out, "graph/task/task_9w4hzd.md: would pin `relies_on[0]` to bel_7k3m9q@r1\n") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if w.Read("task/task_9w4hzd.md") != testws.Task {
		t.Error("a dry run changed a file")
	}
}

func TestSyncSkipsFilesWithErrors(t *testing.T) {
	newWorkspace(t, map[string]string{"task/task_9w4hzd.md": strings.Replace(testws.Task, "status: accepted", "status: acepted", 1)})
	if code, out, _ := mason("sync"); code != 1 || !strings.Contains(out, "graph/task/task_9w4hzd.md: skipped, since it has errors; see `mason validate`\n") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestRevise(t *testing.T) {
	newWorkspace(t, map[string]string{
		"design-doc/dd_c5r1tw.md": "---\nid: dd_c5r1tw\ntitle: Reading files safely\nstatus: accepted\ncreated: 2026-11-22\n---\n\nThe design.\n",
		"task/task_9w4hzd.md":     "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nderived_from: dd_c5r1tw\ncreated: 2026-11-22\n---\n",
	})
	mason("sync")
	// A design doc's old text is left to git, and the workspace isn't in
	// git: the plan's warning goes to stderr.
	code, out, errOut := mason("revise", "dd_c5r1tw", "--non-breaking")
	if code != 0 || !strings.HasPrefix(out, "graph/design-doc/dd_c5r1tw.md: ") || !strings.HasPrefix(errOut, "mason: warning: graph/design-doc/dd_c5r1tw.md: no commit holds") {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
}

func TestReviseMistakes(t *testing.T) {
	newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief})
	if code, _, errOut := mason("revise", "bel_7k3m9q"); code != 2 || !strings.Contains(errOut, "say whether the change is --breaking") {
		t.Errorf("no --breaking or --non-breaking: exit %d, %s", code, errOut)
	}
	if code, _, errOut := mason("revise", "bel_nope00", "--breaking"); code != 1 || errOut != "mason: no record is called `bel_nope00`\n" {
		t.Errorf("no such record: exit %d, %s", code, errOut)
	}
}

func TestSyncDryRunSaysItWouldUnlist(t *testing.T) {
	one := "---\nid: ev_p4n2xa\ntitle: Seen\nobserved_at: 2026-11-22T10:00:00Z\nfrom: []\ngenerated_by: [actv_3h7npc]\ncreated: 2026-11-22\n---\n\nThe words.\n"
	none := strings.Replace(strings.Replace(one, "ev_p4n2xa", "ev_w2q9kd", 1), "[actv_3h7npc]", "[]", 1)
	w := newWorkspace(t, map[string]string{
		"activity/actv_3h7npc.md": "---\nid: actv_3h7npc\ntitle: Session\nsession: a\nstarted_at: 2026-11-22\ncreated: 2026-11-22\n---\n",
		"evidence/ev_p4n2xa.md":   one,
		"evidence/ev_w2q9kd.md":   none,
	})
	code, out, _ := mason("sync", "--dry-run")
	for _, want := range []string{
		"ev_p4n2xa.md: would rewrite `generated_by` from a one-item list to a single value\n",
		"ev_w2q9kd.md: would drop the empty `generated_by` list\n",
	} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("exit %d, missing %q in:\n%s", code, want, out)
		}
	}
	if w.Read("evidence/ev_p4n2xa.md") != one || w.Read("evidence/ev_w2q9kd.md") != none {
		t.Error("a dry run changed a file")
	}
}
