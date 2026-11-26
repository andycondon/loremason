package graph

import (
	"fmt"
	"testing"

	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/record"
)

func TestReadPin(t *testing.T) {
	g := load(t,
		file("bel_7k3m9q"),
		file("ev_m3q8vb"),
		file("task_9w4hzd",
			"relies_on:",
			"  - bel_7k3m9q",
			"  - {to: bel_7k3m9q}",
			"  - {to: bel_7k3m9q, rev: 2}",
			"  - {to: bel_7k3m9q, fp: \"fp1:536221e3130c\"}",
			"  - {to: bel_7k3m9q, rev: 2, fp: \"fp1:536221e3130c\", via: ev_m3q8vb}",
			"  - {to: bel_7k3m9q, rev: 2, fp: \"fp1:536221e3130c\", via: not a ref}",
		),
	)
	var pins []*Pin
	for _, e := range g.Edges {
		if e.Name == "relies_on" {
			pins = append(pins, e.Pin)
		}
	}
	str := func(p *Pin) string {
		if p == nil {
			return "none"
		}
		via := "-"
		if p.Via != nil {
			via = p.Via.String()
		}
		return fmt.Sprintf("%d %s %s", p.Rev, p.FP, via)
	}
	zero := fingerprint.FP{}.String()
	for i, want := range []string{
		"none", // a bare reference
		"none", // {to} alone, like an initiative's {to, role}
		"2 " + zero + " -",
		"0 fp1:536221e3130c -",
		"2 fp1:536221e3130c ev_m3q8vb",
		"2 fp1:536221e3130c -", // a bad via is the schema check's to report
	} {
		if got := str(pins[i]); got != want {
			t.Errorf("relies_on[%d]: got %q, want %q", i, got, want)
		}
	}
}

func TestPinOut(t *testing.T) {
	fp, _ := fingerprint.Parse("fp1:536221e3130c")
	via := Ref{Name: "ev_m3q8vb"}
	for _, c := range []struct {
		pin  Pin
		want string
	}{
		{Pin{Rev: 2, FP: fp}, `{to: old/B-1, rev: 2, fp: "fp1:536221e3130c"}`},
		{Pin{Rev: 2, FP: fp, Via: &via}, `{to: old/B-1, rev: 2, fp: "fp1:536221e3130c", via: ev_m3q8vb}`},
	} {
		if got := record.Render("x", c.pin.Out("old/B-1")); got != "x: "+c.want+"\n" {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
