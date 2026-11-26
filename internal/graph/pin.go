package graph

import (
	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/record"
)

// Pin is what a pinned link records about its target. readPin and Out are
// the one place the form {to, rev, fp, via} is known.
type Pin struct {
	Rev int
	FP  fingerprint.FP
	Via *Ref // the evidence for the last re-check, if there is one
}

// readPin reads the pin in a {to, rev, fp, via} link, or returns nil when
// the link has neither rev nor fp, as an initiative's {to, role} entries
// don't. Bad values are the schema check's to report.
func readPin(m record.Map) *Pin {
	rev, hasRev := m.Get("rev")
	_, hasFP := m.Get("fp")
	if !hasRev && !hasFP {
		return nil
	}
	p := &Pin{}
	p.FP, _ = fingerprint.Parse(m.Text("fp"))
	if hasRev {
		rev.Decode(&p.Rev)
	}
	if via, err := ParseRef(m.Text("via")); err == nil {
		p.Via = &via
	}
	return p
}

// Out writes a link pinned. to is the target as the link names it.
func (p Pin) Out(to string) record.Out {
	fields := []record.KV{
		{Key: "to", Val: record.Str(to)},
		{Key: "rev", Val: record.Int(p.Rev)},
		{Key: "fp", Val: record.Quoted(p.FP.String())},
	}
	if p.Via != nil {
		fields = append(fields, record.KV{Key: "via", Val: record.Str(p.Via.String())})
	}
	return record.FlowMap(fields...)
}
