package main

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// task is a task that relies on the belief testws.Belief.
const task = "---\nid: task_9w4hzd\ntitle: Do it\nstatus: accepted\nrelies_on: [bel_7k3m9q]\ncreated: 2026-11-22\n---\n\nThe task.\n"

func beliefAndTask(t *testing.T) *testws.Workspace {
	t.Helper()
	return newWorkspace(t, map[string]string{"belief/bel_7k3m9q.md": testws.Belief, "task/task_9w4hzd.md": task})
}

func TestFind(t *testing.T) {
	beliefAndTask(t)
	code, out, errOut := mason("find", "utf-8 failures")
	if code != 0 || errOut != "" || out != "bel_7k3m9q  belief  held  validate and encodings\n" {
		t.Errorf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
	if code, out, errOut := mason("find", "nothingmatches"); code != 0 || errOut != "" || out != "" {
		t.Errorf("no match: exit %d, stderr %q, stdout %q", code, errOut, out)
	}
}

func TestFindTakesSeveralWords(t *testing.T) {
	beliefAndTask(t)
	want := "bel_7k3m9q  belief  held  validate and encodings\n"
	for _, args := range [][]string{{"find", "utf-8", "failures"}, {"find", "utf-8 failures"}} {
		if code, out, errOut := mason(args...); code != 0 || errOut != "" || out != want {
			t.Errorf("%v: exit %d, stderr %q, stdout %q", args, code, errOut, out)
		}
	}
}

func TestFindWithNothingToSearchFor(t *testing.T) {
	beliefAndTask(t)
	code, out, errOut := mason("find", "-")
	if code != 0 || out != "" || !strings.Contains(errOut, "no words to search for") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestShowMarksLinksOfSubRecords(t *testing.T) {
	newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"question/q_3fz8km.md": "---\nid: q_3fz8km\nquestion: Which?\noptions:\n  - {id: o1, title: One, for: [bel_7k3m9q]}\ncreated: 2026-11-22\n---\n",
	})
	code, out, errOut := mason("show", "q_3fz8km")
	if code != 0 || !strings.Contains(out, "for [o1]  bel_7k3m9q") || strings.Contains(out, "subrecord_of") {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
}

func TestShow(t *testing.T) {
	beliefAndTask(t)
	mason("sync")
	code, out, errOut := mason("show", "bel_7k3m9q")
	want := "bel_7k3m9q  belief  held  validate and encodings\n" +
		"\nBoth failures are on files that aren't UTF-8.\n" +
		"\nlinks in:\n" +
		"  relies_on  task_9w4hzd  task  accepted  Do it  pinned r1 " + testws.BeliefFP + "\n"
	if code != 0 || errOut != "" || out != want {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
	code, out, _ = mason("show", "task_9w4hzd")
	if code != 0 || !strings.Contains(out, "links out:\n  relies_on  bel_7k3m9q  belief  held  validate and encodings  pinned r1 ") {
		t.Errorf("exit %d, stdout:\n%s", code, out)
	}
}

func TestDependents(t *testing.T) {
	beliefAndTask(t)
	code, out, errOut := mason("dependents", "bel_7k3m9q")
	if code != 0 || errOut != "" || out != "1  task_9w4hzd  task  accepted  Do it  via relies_on of bel_7k3m9q\n" {
		t.Errorf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
}

func TestStale(t *testing.T) {
	w := beliefAndTask(t)
	mason("sync")
	if code, out, _ := mason("stale"); code != 0 || out != "" {
		t.Errorf("nothing changed: exit %d, stdout %q", code, out)
	}
	w.Edit("belief/bel_7k3m9q.md", "validate doesn't handle", "validate mishandles")
	code, out, errOut := mason("stale")
	want := "bel_7k3m9q  belief  held  validate and encodings  r1\n" +
		"  relies_on  task_9w4hzd  task  accepted  Do it  pinned r1 " + testws.BeliefFP + "  (stale)\n"
	// The check counts a stale pin as an error, so the warning is expected.
	if code != 0 || errOut != "mason: warning: the graph has 1 error; run mason validate\n" || out != want {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
}

func TestReopened(t *testing.T) {
	newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": strings.Replace(testws.Belief, "status: held", "status: incorrect", 1),
		"task/task_9w4hzd.md":  task,
		// the same task, turned down: it isn't listed
		"task/task_2b4cdf.md": strings.NewReplacer("task_9w4hzd", "task_2b4cdf", "status: accepted", "status: rejected").Replace(task),
	})
	code, out, errOut := mason("reopened")
	if code != 0 || errOut != "" || out != "task_9w4hzd  task  accepted  Do it  rests on  bel_7k3m9q  validate and encodings  (incorrect)\n" {
		t.Errorf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
}

func TestQueryUnknownID(t *testing.T) {
	beliefAndTask(t)
	for _, cmd := range []string{"show", "dependents"} {
		code, out, errOut := mason(cmd, "bel_7k3m9r")
		if code != 1 || out != "" || errOut != "mason: no record is called `bel_7k3m9r`; did you mean `bel_7k3m9q`?\n" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", cmd, code, out, errOut)
		}
	}
}

func TestQueryUsageMistakes(t *testing.T) {
	beliefAndTask(t)
	for _, args := range [][]string{
		{"find"}, {"show"}, {"show", "a", "b"},
		{"dependents"}, {"dependents", "a", "b"}, {"stale", "x"}, {"reopened", "x"},
	} {
		if code, out, _ := mason(args...); code != 2 || out != "" {
			t.Errorf("%v: exit %d, stdout %q", args, code, out)
		}
	}
}

func TestQueryAnswersWhenTheGraphHasErrors(t *testing.T) {
	newWorkspace(t, map[string]string{
		"belief/bel_7k3m9q.md": testws.Belief,
		"task/task_9w4hzd.md":  strings.Replace(task, "status: accepted", "status: acepted", 1),
	})
	code, out, errOut := mason("find", "utf-8")
	if code != 0 || out != "bel_7k3m9q  belief  held  validate and encodings\n" || errOut != "mason: warning: the graph has 1 error; run mason validate\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}
