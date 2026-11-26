package graph

import (
	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/record"
)

// Revision is one entry in a record's revisions list. readRevisions and Out
// are the one place the entry's form is known.
type Revision struct {
	Rev  int
	Date string
	FP   fingerprint.FP // zero until sync records it
	// Breaking says how this revision compares with the one before: true
	// unless the entry says non_breaking. It means nothing for revision 1.
	Breaking bool
	Commit   string                // for long records, the commit that holds this revision's text
	Kept     map[string]record.Out // the wording kept inline, by meaning field; empty when there's none
	At       Loc
	Implicit bool // not written in the file yet; sync writes it
}

// KeptText is a field's kept wording when it's text, or "".
func (r Revision) KeptText(field string) string {
	w, ok := r.Kept[field]
	if !ok {
		return ""
	}
	s, _ := w.Plain().(string)
	return s
}

// readRevisions reads a revisable record's revisions list. A record whose
// list is missing, empty or has no usable entry is at an implicit revision
// 1. Entries of the wrong shape are skipped; the schema check reports them.
func readRevisions(n *Node) {
	if !n.Revisable() {
		return
	}
	for _, entry := range n.Fields.Entries("revisions") {
		rev, ok := entry.Get("rev")
		r := Revision{
			Date:     entry.Text("date"),
			Breaking: entry.Text("change") != "non_breaking",
			Commit:   entry.Text("commit"),
			Kept:     map[string]record.Out{},
			At:       Loc{Pos: n.File.At(entry.Line()), Field: entry.Path},
		}
		if !ok || rev.Decode(&r.Rev) != nil {
			continue
		}
		r.FP, _ = fingerprint.Parse(entry.Text("fp"))
		for _, field := range n.File.Type.Meaning {
			if v, ok := entry.Get(field); ok {
				r.Kept[field] = v.Out().WithoutComments()
			}
		}
		n.Revisions = append(n.Revisions, r)
	}
	if len(n.Revisions) == 0 {
		n.Revisions = []Revision{{Rev: 1, Breaking: true, Implicit: true, At: Loc{Pos: n.File.At(1)}}}
	}
}

// Out writes the revision as an entry of a revisions list, for a record of
// the node's type.
func (r Revision) Out(n *Node) record.Out {
	fields := []record.KV{{Key: "rev", Val: record.Int(r.Rev)}}
	if r.Date != "" {
		fields = append(fields, record.KV{Key: "date", Val: record.Date(r.Date)})
	}
	if !r.FP.IsZero() {
		fields = append(fields, record.KV{Key: "fp", Val: record.Quoted(r.FP.String())})
	}
	if r.Rev > 1 {
		change := "non_breaking"
		if r.Breaking {
			change = "breaking"
		}
		fields = append(fields, record.KV{Key: "change", Val: record.Str(change)})
	}
	if r.Commit != "" {
		fields = append(fields, record.KV{Key: "commit", Val: record.Str(r.Commit)})
	}
	for _, field := range n.File.Type.Meaning {
		if w, ok := r.Kept[field]; ok {
			fields = append(fields, record.KV{Key: field, Val: w})
		}
	}
	return record.BlockMap(fields...)
}

// KeepsWordingInline reports whether the record's old revisions keep their
// wording in the file (short records), rather than a commit (long ones).
func (n *Node) KeepsWordingInline() bool { return n.File.Type.RevisionsInline }

// Wording is the record's meaning as it is now, without comments: what a
// revision keeps inline when the record is revised.
func (n *Node) Wording() map[string]record.Out {
	w := map[string]record.Out{}
	for _, field := range n.File.Type.MeaningFields() {
		if v, ok := n.Fields.Get(field); ok {
			w[field] = v.Out().WithoutComments()
		}
	}
	if n.File.Type.BodyIsMeaning() && n.File.Record != nil {
		w["body"] = record.Str(fingerprint.NormalizeBody(n.File.Record.Body) + "\n")
	}
	return w
}

// Fingerprint is the fingerprint of the record's meaning as it is now. It's
// zero for sub-records, and for files that couldn't be read.
func (n *Node) Fingerprint() fingerprint.FP {
	if n.IsSubrecord() || n.File.Record == nil {
		return fingerprint.FP{}
	}
	fields := map[string]any{}
	for _, name := range n.File.Type.MeaningFields() {
		if v, ok := n.Fields.Get(name); ok {
			fields[name] = withoutRevisions(v.Plain())
		}
	}
	return fingerprint.Of(fields, n.File.Record.Body, n.File.Type.BodyIsMeaning())
}

// KeptFingerprint is the fingerprint of a revision's kept wording. ok is
// false when the revision doesn't keep all of its meaning inline.
func (n *Node) KeptFingerprint(r Revision) (fp fingerprint.FP, ok bool) {
	fields := map[string]any{}
	for _, name := range n.File.Type.MeaningFields() {
		w, ok := r.Kept[name]
		if !ok {
			return fingerprint.FP{}, false
		}
		fields[name] = withoutRevisions(w.Plain())
	}
	body := ""
	if n.File.Type.BodyIsMeaning() {
		w, ok := r.Kept["body"]
		if !ok {
			return fingerprint.FP{}, false
		}
		body, _ = w.Plain().(string)
	}
	return fingerprint.Of(fields, body, n.File.Type.BodyIsMeaning()), true
}

// withoutRevisions writes references in a value without their @rN, so that
// moving a link to a newer revision doesn't change what the record means.
func withoutRevisions(v any) any {
	switch v := v.(type) {
	case string:
		if r, err := ParseRef(v); err == nil && r.Rev > 0 {
			return r.Name
		}
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = withoutRevisions(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = withoutRevisions(item)
		}
		return out
	}
	return v
}
