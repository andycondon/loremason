package fingerprint

import (
	"testing"
)

// Each normalisation rule in section 5, with a body that differs from the
// base only by that rule.
func TestFormattingDoesNotChangeTheFingerprint(t *testing.T) {
	base := "First line.\n\nSecond paragraph,\nstill second."
	for rule, body := range map[string]string{
		"Windows line endings":              "First line.\r\n\r\nSecond paragraph,\r\nstill second.",
		"trailing spaces and tabs":          "First line.  \n\nSecond paragraph,\t\nstill second. ",
		"runs of blank lines":               "First line.\n\n\n\nSecond paragraph,\nstill second.",
		"blank lines at the start":          "\n\nFirst line.\n\nSecond paragraph,\nstill second.",
		"blank lines at the end":            "First line.\n\nSecond paragraph,\nstill second.\n\n\n",
		"blank lines that hold only spaces": "First line.\n   \nSecond paragraph,\nstill second.",
	} {
		if Of(nil, body, true) != Of(nil, base, true) {
			t.Errorf("%s changed the fingerprint", rule)
		}
	}
}

func TestMeaningChangesTheFingerprint(t *testing.T) {
	base := Of(map[string]any{"claim": "a"}, "body", true)
	for what, fp := range map[string]FP{
		"a field's value":                 Of(map[string]any{"claim": "b"}, "body", true),
		"a word in the body":              Of(map[string]any{"claim": "a"}, "Body", true),
		"leading spaces in a line":        Of(map[string]any{"claim": "a"}, "  body", true),
		"the body, when it isn't meaning": Of(map[string]any{"claim": "a"}, "", false),
	} {
		if fp == base {
			t.Errorf("changing %s didn't change the fingerprint", what)
		}
	}
	if Of(map[string]any{"claim": "a"}, "x", false) != Of(map[string]any{"claim": "a"}, "y", false) {
		t.Error("the body changed the fingerprint of a record whose body isn't meaning")
	}
}

// The canonical form is pinned down exactly: sorted keys, compact JSON,
// UTF-8, and no HTML escaping.
func TestCanonical(t *testing.T) {
	got := string(Canonical(map[string]any{"title": "a < b & c", "criteria": []any{"é", 2}}, "x\n", true))
	want := `{"body":"x","criteria":["é",2],"title":"a < b & c"}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// Known values, computed independently (Python's hashlib), so an
	// accidental change of method shows up.
	for want, fp := range map[string]FP{
		"fp1:536221e3130c": Of(map[string]any{"claim": "validate doesn't handle non-UTF-8 input"}, "", false),
		"fp1:ea56a4932c11": Of(map[string]any{"title": "a < b & c", "criteria": []any{"é", 2}}, "x\n", true),
	} {
		if fp.String() != want {
			t.Errorf("got %s, want %s", fp, want)
		}
	}
}

func TestParse(t *testing.T) {
	fp, err := Parse("fp1:3a9c0e1b77d2")
	if err != nil || fp.Method != 1 || fp.Hex != "3a9c0e1b77d2" || fp.String() != "fp1:3a9c0e1b77d2" {
		t.Errorf("got %+v, %v", fp, err)
	}
	for _, bad := range []string{"", "fp1:", "fp0:3a9c0e1b77d2", "fp1:3A9C0E1B77D2", "fp1:3a9c0e1b77d", "3a9c0e1b77d2"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) worked", bad)
		}
	}
}
