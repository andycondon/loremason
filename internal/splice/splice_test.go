package splice

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const input = `---
id: task_9w4hzd
title: "Report unreadable files clearly: name the file"
status: accepted   # agreed 2026-11-22
implements:
  - {to: req_5txk3a, rev: 1, fp: "fp1:7d0e2a91c4b3"}
derived_from:
  # the design doc this came from
  - dd_c5r1tw

  # a note after the list, before the next field
relies_on: [bel_7k3m9q, bel_x2b9k4]
created: 2026-11-22
extra:
  confidence: 0.7   # experiment
  notes: >
    A folded block that someone wrote by hand,
    spread over two lines.
links_last:
  - a
---

## Outcome
validate names any file it can't read.

  Indented line, trailing spaces kept.   
`

func edit(t *testing.T, src string, sets map[string]string) string {
	t.Helper()
	h, err := Edit([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for f, text := range sets {
		h.Set(f, text)
	}
	out := string(h.Bytes())
	header := strings.SplitN(out, "\n---", 2)[0]
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(header, "---")), &n); err != nil {
		t.Fatalf("the result doesn't parse: %v\n%s", err, out)
	}
	return out
}

func TestReplaceKeepsEverythingElse(t *testing.T) {
	out := edit(t, input, map[string]string{
		"relies_on":    "relies_on:\n  - {to: bel_7k3m9q, rev: 1, fp: \"fp1:3a9c0e1b77d2\"}\n  - bel_x2b9k4\n",
		"derived_from": "derived_from:\n  # the design doc this came from\n  - {to: dd_c5r1tw, rev: 2, fp: \"fp1:e81f5530a2d9\"}\n",
		"links_last":   "links_last:\n  - b\n",
	})
	want := strings.NewReplacer(
		"relies_on: [bel_7k3m9q, bel_x2b9k4]\n", "relies_on:\n  - {to: bel_7k3m9q, rev: 1, fp: \"fp1:3a9c0e1b77d2\"}\n  - bel_x2b9k4\n",
		"  - dd_c5r1tw\n", "  - {to: dd_c5r1tw, rev: 2, fp: \"fp1:e81f5530a2d9\"}\n",
		"links_last:\n  - a\n", "links_last:\n  - b\n",
	).Replace(input)
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestNewFieldGoesBeforeCreated(t *testing.T) {
	out := edit(t, input, map[string]string{"revisions": "revisions:\n  - rev: 1\n"})
	if !strings.Contains(out, "relies_on: [bel_7k3m9q, bel_x2b9k4]\nrevisions:\n  - rev: 1\ncreated: 2026-11-22\n") {
		t.Errorf("got:\n%s", out)
	}
	noCreated := "---\nid: x\n---\nbody\n"
	if got := edit(t, noCreated, map[string]string{"revisions": "revisions: []\n"}); got != "---\nid: x\nrevisions: []\n---\nbody\n" {
		t.Errorf("got %q", got)
	}
}

func TestWindowsLineEndingsAreKept(t *testing.T) {
	src := strings.ReplaceAll(input, "\n", "\r\n")
	out := edit(t, src, map[string]string{"relies_on": "relies_on:\n  - a\n"})
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") {
		t.Errorf("mixed line endings:\n%q", out)
	}
}

func TestNothingSetChangesNothing(t *testing.T) {
	if out := edit(t, input, nil); out != input {
		t.Error("an edit with no changes changed the file")
	}
}

func TestEditRejectsWhatItCantRead(t *testing.T) {
	for _, src := range []string{"", "no header\n", "---\nid: x\n", "---\n- a list\n---\n", "---\na: [\n---\n"} {
		if _, err := Edit([]byte(src)); err == nil {
			t.Errorf("Edit(%q) worked", src)
		}
	}
}

func TestByteOrderMarkIsKept(t *testing.T) {
	h, err := Edit([]byte("\ufeff---\nid: x\nrelies_on: [a]\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	h.Set("relies_on", "relies_on:\n  - b\n")
	if got := string(h.Bytes()); got != "\ufeff---\nid: x\nrelies_on:\n  - b\n---\n" {
		t.Errorf("got %q", got)
	}
}

func TestNewFieldGoesAboveTheCommentOnCreated(t *testing.T) {
	src := "---\nid: x\n# when it was written\ncreated: 2026-11-22\n---\n"
	if got := edit(t, src, map[string]string{"revisions": "revisions: []\n"}); got != "---\nid: x\nrevisions: []\n# when it was written\ncreated: 2026-11-22\n---\n" {
		t.Errorf("got %q", got)
	}
}

func TestRemoveDeletesTheFieldsLines(t *testing.T) {
	h, err := Edit([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	h.Remove("relies_on")
	h.Remove("links_last")
	h.Remove("not_there")
	out := string(h.Bytes())
	if strings.Contains(out, "relies_on") || strings.Contains(out, "links_last") || strings.Contains(out, "not_there") {
		t.Errorf("fields remain:\n%s", out)
	}
	for _, keep := range []string{"status: accepted   # agreed 2026-11-22", "created: 2026-11-22", "notes: >", "Indented line"} {
		if !strings.Contains(out, keep) {
			t.Errorf("lost %q:\n%s", keep, out)
		}
	}
}
