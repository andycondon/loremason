package record

import "testing"

func TestRewriteKeepsComments(t *testing.T) {
	r, errs := Parse([]byte(`---
flow: [a, b]   # on a flow list
single: a   # on a single value
block:   # on a block list
  # before an item
  - a   # after an item
---
`))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for field, want := range map[string]string{
		"flow":   "flow: # on a flow list\n  - a\n  - b\n",
		"single": "single:\n  - a # on a single value\n",
		"block":  "block: # on a block list\n  # before an item\n  - a # after an item\n",
	} {
		v, _ := r.Header.Get(field)
		var items []Out
		for _, item := range v.Items() {
			items = append(items, item.Out())
		}
		if got := v.Rewrite(List(items...)); got != want {
			t.Errorf("%s: got %q, want %q", field, got, want)
		}
	}
}

func TestRenderAndQuote(t *testing.T) {
	got := Render("revisions", List(BlockMap(KV{"rev", Int(1)}, KV{"date", Date("2026-11-22")}, KV{"fp", Quoted("fp1:3a9c0e1b77d2")}, KV{"claim", Str("it's: odd")})))
	want := "revisions:\n  - rev: 1\n    date: 2026-11-22\n    fp: \"fp1:3a9c0e1b77d2\"\n    claim: 'it''s: odd'\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	for in, want := range map[string]string{"plain": "plain", "a: b": "'a: b'", "2026-11-22": `"2026-11-22"`, "true": `"true"`} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}
