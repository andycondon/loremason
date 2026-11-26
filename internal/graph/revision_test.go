package graph

import (
	"testing"

	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/record"
)

// ---- Reading and writing a revisions list ----

func TestReadRevisions(t *testing.T) {
	bel := load(t, file("bel_7k3m9q",
		"claim: a new claim",
		"revisions:",
		"  - rev: 1",
		"    date: 2026-11-22",
		"    fp: \"fp1:536221e3130c\"",
		"    claim: an old claim   # a note",
		"  - {date: 2026-11-23}",
		"  - {rev: 2, change: non_breaking}",
		"  - {rev: 3, change: breaking}",
		"  - {rev: 4}",
	)).Node("bel_7k3m9q")
	// The entry with no rev is skipped: the schema check reports it.
	if len(bel.Revisions) != 4 {
		t.Fatalf("revisions: %+v", bel.Revisions)
	}
	r1 := bel.Revisions[0]
	if r1.Rev != 1 || r1.Date != "2026-11-22" || r1.FP.String() != "fp1:536221e3130c" || r1.KeptText("claim") != "an old claim" || r1.Implicit {
		t.Errorf("revision 1: %+v", r1)
	}
	if r1.At.Line != 6 || r1.At.Field.String() != "revisions[0]" {
		t.Errorf("revision 1 is at %+v", r1.At)
	}
	// change says how a revision compares with the one before; missing means breaking.
	for i, want := range []bool{true, false, true, true} {
		if bel.Revisions[i].Breaking != want {
			t.Errorf("revision %d: Breaking %v", bel.Revisions[i].Rev, !want)
		}
	}
}

func TestReadRevisionsImplicitFirst(t *testing.T) {
	for name, lines := range map[string][]string{
		"no list":         nil,
		"an empty list":   {"revisions: []"},
		"no usable entry": {"revisions:", "  - {date: 2026-11-22}", "  - {rev: x}"},
	} {
		bel := load(t, file("bel_7k3m9q", lines...)).Node("bel_7k3m9q")
		if len(bel.Revisions) != 1 || bel.Current().Rev != 1 || !bel.Current().Implicit || !bel.Current().Breaking {
			t.Errorf("%s: %+v", name, bel.Revisions)
		}
	}
	if actor := load(t, file("who_4d9snc")).Node("who_4d9snc"); actor.Revisions != nil {
		t.Error("an actor keeps no revisions, not even an implicit one")
	}
}

func TestRevisionOut(t *testing.T) {
	bel := load(t, file("bel_7k3m9q", "claim: a claim")).Node("bel_7k3m9q")
	fp, _ := fingerprint.Parse("fp1:536221e3130c")
	revs := []Revision{
		{Rev: 1, Date: "2026-11-22", FP: fp, Breaking: true, Kept: map[string]record.Out{"claim": record.Str("an old claim")}},
		{Rev: 2, Date: "2026-11-23", Breaking: false, Commit: "4f2a9c1"},
		{Rev: 3, Breaking: true},
	}
	var outs []record.Out
	for _, r := range revs {
		outs = append(outs, r.Out(bel))
	}
	got := record.Render("revisions", record.List(outs...))
	// Revision 1 says nothing about change; an unset date or fingerprint is left out.
	want := "revisions:\n" +
		"  - rev: 1\n    date: 2026-11-22\n    fp: \"fp1:536221e3130c\"\n    claim: an old claim\n" +
		"  - rev: 2\n    date: 2026-11-23\n    change: non_breaking\n    commit: 4f2a9c1\n" +
		"  - rev: 3\n    change: breaking\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// What Out writes, readRevisions reads back.
	back := load(t, file("bel_7k3m9q", "claim: a claim", got[:len(got)-1])).Node("bel_7k3m9q").Revisions
	for i, r := range back {
		w := revs[i]
		if r.Rev != w.Rev || r.Date != w.Date || r.FP != w.FP || r.Breaking != w.Breaking || r.Commit != w.Commit || r.KeptText("claim") != w.KeptText("claim") {
			t.Errorf("revision %d read back as %+v", w.Rev, r)
		}
	}
}

func TestKeptText(t *testing.T) {
	r := Revision{Kept: map[string]record.Out{"claim": record.Str("a claim"), "count": record.Int(3)}}
	if r.KeptText("claim") != "a claim" || r.KeptText("count") != "" || r.KeptText("missing") != "" {
		t.Error("KeptText is a kept field's text, or empty")
	}
}

// ---- A record's meaning ----

func TestKeepsWordingInline(t *testing.T) {
	g := load(t, file("bel_7k3m9q"), file("dd_c5r1tw"))
	if !g.Node("bel_7k3m9q").KeepsWordingInline() || g.Node("dd_c5r1tw").KeepsWordingInline() {
		t.Error("a belief keeps its old wording; a design doc leaves it to git")
	}
}

func TestWording(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "claim: a claim   # a note"),
		"---\nid: step_2c7rq1\ntitle: Catch it\ncreated: 2026-11-22\n---\n\nThe body.   \n\n\n",
	)
	w := g.Node("bel_7k3m9q").Wording()
	if len(w) != 1 || record.Render("claim", w["claim"]) != "claim: a claim\n" {
		t.Errorf("a belief's wording is its claim, without the comment: %v", w)
	}
	w = g.Node("step_2c7rq1").Wording()
	if len(w) != 2 || w["title"].Plain() != "Catch it" || w["body"].Plain() != "The body.\n" {
		t.Errorf("a step's wording is its title and normalised body: %v", w)
	}
}

func TestFingerprint(t *testing.T) {
	fp := func(lines ...string) fingerprint.FP {
		return load(t, file("bel_7k3m9q", lines...)).Node("bel_7k3m9q").Fingerprint()
	}
	base := fp("claim: a claim", "status: held")
	if base.String() != fingerprint.Of(map[string]any{"claim": "a claim"}, "", false).String() {
		t.Errorf("a belief's fingerprint covers its claim alone: %s", base)
	}
	if fp("claim: a claim", "status: withdrawn") != base {
		t.Error("status isn't part of a belief's meaning")
	}
	if fp("claim: another claim", "status: held") == base {
		t.Error("the claim is")
	}
	g, _ := loadFiles(t, map[string]string{"belief/bel_7k3m9q.md": "no header\n"})
	if !g.Node("bel_7k3m9q").Fingerprint().IsZero() {
		t.Error("a file that can't be read has no fingerprint")
	}
	sub := load(t, file("bel_7k3m9q", "support:", "  - {id: s1}")).Node("bel_7k3m9q/s1")
	if !sub.Fingerprint().IsZero() {
		t.Error("a sub-record has no fingerprint of its own")
	}
}

func TestKeptFingerprint(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q", "claim: a claim"),
		"---\nid: step_2c7rq1\ntitle: Catch it\ncreated: 2026-11-22\n---\n\nThe body.\n",
	)
	// Wording kept now matches the fingerprint now.
	for _, id := range []string{"bel_7k3m9q", "step_2c7rq1"} {
		n := g.Node(id)
		if fp, ok := n.KeptFingerprint(Revision{Kept: n.Wording()}); !ok || fp != n.Fingerprint() {
			t.Errorf("%s: %s, %v; now %s", id, fp, ok, n.Fingerprint())
		}
	}
	step := g.Node("step_2c7rq1")
	if _, ok := step.KeptFingerprint(Revision{Kept: map[string]record.Out{"title": record.Str("Catch it")}}); ok {
		t.Error("wording kept without the body isn't all of a step's meaning")
	}
}

func TestWithoutRevisions(t *testing.T) {
	in := []any{"bel_7k3m9q@r2", "plain text", map[string]any{"to": "q_3fz8km/o1@r1"}, 3}
	got := withoutRevisions(in).([]any)
	if got[0] != "bel_7k3m9q" || got[1] != "plain text" || got[2].(map[string]any)["to"] != "q_3fz8km/o1" || got[3] != 3 {
		t.Errorf("got %v", got)
	}
	if in[0] != "bel_7k3m9q@r2" {
		t.Error("the input was changed")
	}
}
