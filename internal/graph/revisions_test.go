package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/registry"
)

// workspace writes records into a temporary workspace and loads it.
func workspace(t *testing.T, files map[string]string) *Graph {
	t.Helper()
	root := t.TempDir()
	for rel, text := range files {
		p := filepath.Join(root, Dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	g, ds, err := Load(root, registry.Load())
	if err != nil || len(ds) > 0 {
		t.Fatalf("%v %v", err, ds)
	}
	return g
}

const belief = `---
id: bel_7k3m9q
title: validate and encodings
claim: %s
status: held
asserted_by: agent
revisions:
  - {rev: 1, date: 2026-11-22, fp: "%s", claim: validate doesn't handle non-UTF-8 input}
  - {rev: 2, date: 2026-11-29, fp: "%s", change: non_breaking}
created: 2026-11-22
---

Reasoning that isn't part of the meaning.
`

const task = `---
id: task_9w4hzd
title: Report unreadable files clearly
status: accepted
relies_on:
  - {to: bel_7k3m9q, rev: %d, fp: "%s"}
created: 2026-11-22
---
`

// fpOf loads a belief with the claim and returns its fingerprint.
func fpOf(t *testing.T, claim string) string {
	g := workspace(t, map[string]string{"belief/bel_7k3m9q.md": fmt.Sprintf(belief, claim, "fp1:000000000000", "fp1:000000000000")})
	return g.Node("bel_7k3m9q").Fingerprint().String()
}

func TestRevisionsPinsAndStaleness(t *testing.T) {
	claim := "validate doesn't handle Latin-1 input"
	fp := fpOf(t, claim)
	write := func(rev int, pinFP string) *Graph {
		return workspace(t, map[string]string{
			"belief/bel_7k3m9q.md": fmt.Sprintf(belief, claim, "fp1:111111111111", fp),
			"task/task_9w4hzd.md":  fmt.Sprintf(task, rev, pinFP),
		})
	}

	g := write(2, fp)
	bel, e := g.Node("bel_7k3m9q"), g.Edges[0]
	if len(bel.Revisions) != 2 || bel.Current().Rev != 2 || bel.Current().Breaking || bel.Current().Date != "2026-11-29" {
		t.Fatalf("revisions: %+v", bel.Revisions)
	}
	// A missing change value counts as breaking.
	if r1 := bel.Revision(1); r1 == nil || !r1.Breaking || r1.Old.Text("claim") != "validate doesn't handle non-UTF-8 input" {
		t.Errorf("revision 1: %+v", r1)
	}
	if e.Target != bel || len(bel.Incoming) != 1 || bel.Incoming[0] != e {
		t.Errorf("the edge isn't resolved both ways")
	}
	if !bel.Sealed(2) || bel.Sealed(1) {
		t.Error("only the pinned revision is sealed")
	}
	if e.Stale() || len(e.Behind()) != 0 {
		t.Error("a pin to the current revision and fingerprint isn't stale")
	}

	g = write(1, "fp1:111111111111")
	e = g.Edges[0]
	if !e.Stale() || len(e.Behind()) != 1 || e.Behind()[0].Rev != 2 {
		t.Errorf("a pin to an older revision is stale and one behind: %v %+v", e.Stale(), e.Behind())
	}

	g = write(2, "fp1:222222222222")
	if !g.Edges[0].Stale() {
		t.Error("a pin whose fingerprint differs is stale")
	}
}

func TestFingerprintIgnoresWhatIsNotMeaning(t *testing.T) {
	fp := fpOf(t, "a claim")
	g := workspace(t, map[string]string{"belief/bel_7k3m9q.md": fmt.Sprintf(strings.Replace(belief, "Reasoning that", "Re-wrapped reasoning\nthat", 1), "a claim", "fp1:000000000000", "fp1:000000000000")})
	if got := g.Node("bel_7k3m9q").Fingerprint().String(); got != fp {
		t.Errorf("a belief's body and title aren't its meaning, but the fingerprint changed: %s vs %s", got, fp)
	}
	if withoutRevisions([]any{"bel_7k3m9q@r2", map[string]any{"to": "q_3fz8km/o1@r1"}, 3}).([]any)[0] != "bel_7k3m9q" {
		t.Error("references in meaning fields keep their @rN")
	}
}
