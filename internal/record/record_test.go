package record

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		src := strings.ReplaceAll("---\nid: bel_7k3m9q\ncreated: 2026-11-22\nsupport:\n  - {id: s1}\n  - id: s2\n    rev: 2\n---\n\nBody\n---\nmore\n", "\n", eol)
		r, errs := Parse([]byte(src))
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if r.Fields["created"] != "2026-11-22" {
			t.Errorf("created is %#v", r.Fields["created"])
		}
		if want := strings.ReplaceAll("\nBody\n---\nmore\n", "\n", eol); r.Body != want {
			t.Errorf("body is %q", r.Body)
		}
		if got := r.Line("support", "1", "rev"); got != 7 {
			t.Errorf("rev is on line %d", got)
		}
		if got := r.Line("support", "1", "nope"); got != 6 {
			t.Errorf("a missing field points at line %d", got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for src, want := range map[string]string{
		"":                           "line 1: the file must start",
		"---\nid: x\n":               "line 1: the file must start",
		"hello\n---\n---\n":          "line 1: the file must start",
		"---\n---\n":                 "line 1: the header must be",
		"---\n- a\n---\n":            "line 1: the header must be",
		"---\na: 1\nb: [\n---\n":     "the header isn't valid YAML",
		"---\na: 1\na: 2\n---\n":     "line 3: `a` appears twice",
		"---\na: &x 1\nb: *x\n---\n": "line 2: YAML anchors",
		"---\na: 1\nb: \xff\n---\n":  "line 3: the file isn't valid UTF-8",
	} {
		_, errs := Parse([]byte(src))
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), want) {
			t.Errorf("%q: got %v, want %q", src, errs, want)
		}
	}
}
