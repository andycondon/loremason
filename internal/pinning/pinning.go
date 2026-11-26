// Package pinning keeps links pinned to what they were built against. It
// works in two steps: Plan decides what would change and says why, and
// Apply writes it. The design is in docs/context-graph/03-implementation.md,
// sections 4 and 5.
package pinning

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/splice"
)

// Action is one thing a plan does to a file, said for people.
type Action struct {
	Path string
	What string
}

func (a Action) String() string { return a.Path + ": " + a.What }

// Plan is what sync or revise would do.
type Plan struct {
	Actions []Action
	Skipped []string // files left alone because they have errors

	edits map[*graph.File]*fileEdit
}

// fileEdit is the new state of one file's tool-owned fields.
type fileEdit struct {
	file      *graph.File
	revisions []revision                 // the whole list, when it changes
	pins      map[*graph.Edge]*graph.Pin // links to write pinned, by the edge read from the file
}

// revision is a revision as it will be written.
type revision struct {
	graph.Revision
	// wording holds the old wording to keep inline, by meaning field. For a
	// revision already in the file, it's read from the entry itself.
	wording map[string]record.Out
}

// Empty reports whether the plan changes nothing.
func (p *Plan) Empty() bool { return len(p.Actions) == 0 }

func newPlan() *Plan { return &Plan{edits: map[*graph.File]*fileEdit{}} }

func (p *Plan) edit(f *graph.File) *fileEdit {
	e := p.edits[f]
	if e == nil {
		e = &fileEdit{file: f, pins: map[*graph.Edge]*graph.Pin{}}
		p.edits[f] = e
	}
	return e
}

func (p *Plan) say(f *graph.File, format string, args ...any) {
	p.Actions = append(p.Actions, Action{f.Path, fmt.Sprintf(format, args...)})
}

// Sync plans what `mason sync` does, in an order that matters, because a pin
// seals the revision it names:
//
//  1. write revision 1 for each revisable record that has no revisions yet;
//  2. record the fingerprint of each current revision nothing pins yet, so
//     drafts can change freely;
//  3. pin each bare link whose edge is pinned;
//  4. re-pin a link whose target's later revisions are all non-breaking.
//
// Files in broken (with errors) are skipped, and so are links to them.
func Sync(g *graph.Graph, broken map[string]bool, today string) *Plan {
	p := newPlan()
	revs := map[*graph.Node][]revision{} // each record's revisions after steps 1 and 2
	for _, f := range g.Files {
		if broken[f.Path] {
			p.Skipped = append(p.Skipped, f.Path)
			continue
		}
		if f.Node.Revisable() {
			revs[f.Node] = p.recordRevision(f, today)
		}
	}
	for _, e := range g.Edges {
		if e.Target == nil || e.From.IsSubrecord() || broken[e.From.File.Path] || broken[e.Target.File.Path] || !e.Def.ShouldPin(e.Target.Kind) {
			continue
		}
		p.pin(e, revs[e.Target])
	}
	return p
}

// recordRevision does steps 1 and 2 for one record, and returns its
// revisions as they will be.
func (p *Plan) recordRevision(f *graph.File, today string) []revision {
	n := f.Node
	rs := existing(n)
	cur := &rs[len(rs)-1]
	fp := n.Fingerprint()
	switch {
	case cur.Implicit:
		cur.Implicit, cur.Date, cur.FP = false, today, fp
		p.say(f, "write revision 1")
	case cur.FP.IsZero():
		cur.FP = fp
		p.say(f, "record the fingerprint of revision %d", cur.Rev)
	case cur.FP != fp && !n.Sealed(cur.Rev):
		cur.FP = fp
		p.say(f, "refresh the fingerprint of revision %d, which nothing pins yet", cur.Rev)
	default:
		return rs
	}
	p.edit(f).revisions = rs
	return rs
}

// pin does steps 3 and 4 for one link.
func (p *Plan) pin(e *graph.Edge, target []revision) {
	if len(target) == 0 {
		return
	}
	f, at := e.From.File, e.Item.Path
	cur := target[len(target)-1]
	switch {
	case e.Pin == nil:
		// A bare link pins what it names: a revision written as @rN, or
		// else the current one.
		r := cur
		if e.To.Rev > 0 {
			i := slices.IndexFunc(target, func(r revision) bool { return r.Rev == e.To.Rev })
			if i < 0 {
				return // check reports a revision that doesn't exist
			}
			r = target[i]
		}
		p.edit(f).pins[e] = &graph.Pin{Rev: r.Rev, FP: r.FP}
		p.say(f, "pin `%s` to %s@r%d", at, e.Target.ID, r.Rev)
	case e.Pin.Rev < cur.Rev && allNonBreaking(target, e.Pin.Rev):
		p.edit(f).pins[e] = &graph.Pin{Rev: cur.Rev, FP: cur.FP} // the re-check it recorded no longer applies
		p.say(f, "re-pin `%s` to %s@r%d: every revision since r%d is non-breaking", at, e.Target.ID, cur.Rev, e.Pin.Rev)
	}
}

func allNonBreaking(revs []revision, after int) bool {
	for _, r := range revs {
		if r.Rev > after && r.Breaking {
			return false
		}
	}
	return true
}

func existing(n *graph.Node) []revision {
	rs := make([]revision, len(n.Revisions))
	for i, r := range n.Revisions {
		rs[i] = revision{Revision: r}
	}
	return rs
}

// Revise plans `mason revise`: it keeps the current revision's wording (or,
// for long records, the commit holding it) and opens a new revision. The
// edit comes after, and the next sync records the new revision's
// fingerprint. commit may be empty when it isn't known.
func Revise(n *graph.Node, breaking bool, today, commit string) (*Plan, error) {
	if !n.Revisable() {
		return nil, fmt.Errorf("%s is of type %s, which has no revisions", n.ID, n.Kind.Name)
	}
	f := n.File
	if f.Record == nil {
		return nil, fmt.Errorf("%s can't be read; run `mason validate`", f.Path)
	}
	rs := existing(n)
	cur := &rs[len(rs)-1]
	fp := n.Fingerprint()
	switch {
	case !n.Sealed(cur.Rev):
		return nil, fmt.Errorf("nothing pins revision %d of %s yet, so it can be edited as it is; no new revision is needed", cur.Rev, n.ID)
	case !cur.FP.IsZero() && cur.FP != fp:
		return nil, fmt.Errorf("the meaning of %s already changed after revision %d was pinned; undo the change first, so the old wording can be kept", n.ID, cur.Rev)
	}
	if cur.Implicit {
		cur.Implicit, cur.Date, cur.FP = false, today, fp
	}
	if f.Type.RevisionsInline {
		cur.wording = wordingNow(n)
	} else if commit != "" {
		cur.Commit = commit
	}
	rs = append(rs, revision{Revision: graph.Revision{Rev: cur.Rev + 1, Date: today, FP: fp, Breaking: breaking}})
	p := newPlan()
	p.edit(f).revisions = rs
	kind := "non-breaking"
	if breaking {
		kind = "breaking"
	}
	p.say(f, "open revision %d (%s); make the edit now, then run `mason sync`", cur.Rev+1, kind)
	return p, nil
}

// Recheck records the outcome "still holds" for a stale link: its pin moves
// to the target's current revision, with via naming the evidence that says
// why. The record holding the link gets no new revision, so nothing built on
// it is touched (early cutoff). The re-check skill calls this.
func Recheck(e *graph.Edge, evidence *graph.Node) (*Plan, error) {
	switch {
	case e.Pin == nil || e.Target == nil:
		return nil, fmt.Errorf("`%s` in %s isn't a pinned link", e.Item.Path, e.From.File.Path)
	case evidence.Kind.Name != "evidence":
		return nil, fmt.Errorf("%s is of type %s; the reason for a re-check is evidence", evidence.ID, evidence.Kind.Name)
	}
	cur := e.Target.Current()
	if cur == nil || cur.FP.IsZero() {
		return nil, fmt.Errorf("%s has no recorded revision to pin yet; run `mason sync`", e.Target.ID)
	}
	p := newPlan()
	p.edit(e.From.File).pins[e] = &graph.Pin{Rev: cur.Rev, FP: cur.FP, Via: &graph.Edge{Name: "via", To: graph.Ref{Name: evidence.ID}}}
	p.say(e.From.File, "re-pin `%s` to %s@r%d: it still holds, as %s says", e.Item.Path, e.Target.ID, cur.Rev, evidence.ID)
	return p, nil
}

// wordingNow is a record's meaning fields as they are now, to keep inline.
func wordingNow(n *graph.Node) map[string]record.Out {
	w := map[string]record.Out{}
	for _, field := range n.File.Type.Meaning {
		if field == "body" {
			w[field] = record.Str(fingerprint.NormalizeBody(n.File.Record.Body) + "\n")
		} else if v, ok := n.Fields.Get(field); ok {
			w[field] = v.Out()
		}
	}
	return w
}

// Apply writes a plan's changes under root.
func Apply(root string, p *Plan) error {
	for _, f := range slices.SortedFunc(maps.Keys(p.edits), func(a, b *graph.File) int { return strings.Compare(a.Path, b.Path) }) {
		if err := p.edits[f].write(root); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	return nil
}

func (e *fileEdit) write(root string) error {
	path := filepath.Join(root, filepath.FromSlash(e.file.Path))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	h, err := splice.Edit(data)
	if err != nil {
		return err
	}
	if e.revisions != nil {
		if v, ok := e.file.Node.Fields.Get("revisions"); ok {
			h.Set("revisions", v.Rewrite(e.renderRevisions()))
		} else {
			h.Set("revisions", record.Render("revisions", e.renderRevisions()))
		}
	}
	for _, field := range e.pinnedFields() {
		v, _ := e.file.Node.Fields.Get(field)
		h.Set(field, v.Rewrite(e.renderLinks(v)))
	}
	return writeFile(path, h.Bytes())
}

// pinnedFields lists the fields that hold a link being pinned.
func (e *fileEdit) pinnedFields() []string {
	var fields []string
	for edge := range e.pins {
		if !slices.Contains(fields, edge.Name) {
			fields = append(fields, edge.Name)
		}
	}
	slices.Sort(fields)
	return fields
}

// renderLinks writes a link field as a block list, pinning the links in the
// plan and leaving the rest as they were.
func (e *fileEdit) renderLinks(field record.Value) record.Out {
	pinned := map[string]*graph.Pin{} // by the link's path in the header
	names := map[string]string{}
	for edge, pin := range e.pins {
		pinned[edge.Item.Path.String()] = pin
		names[edge.Item.Path.String()] = edge.To.Name
	}
	var items []record.Out
	for _, item := range field.Items() {
		pin, ok := pinned[item.Path.String()]
		if !ok {
			items = append(items, item.Out())
			continue
		}
		link := []record.KV{{Key: "to", Val: record.Str(names[item.Path.String()])}, {Key: "rev", Val: record.Int(pin.Rev)}, {Key: "fp", Val: record.Quoted(pin.FP.String())}}
		if pin.Via != nil {
			link = append(link, record.KV{Key: "via", Val: record.Str(pin.Via.To.String())})
		}
		items = append(items, record.FlowMap(link...).WithCommentsOf(item))
	}
	return record.List(items...)
}

// renderRevisions writes the revisions list.
func (e *fileEdit) renderRevisions() record.Out {
	var entries []record.Out
	for _, r := range e.revisions {
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
		for _, m := range e.file.Type.Meaning {
			if w, ok := r.wording[m]; ok {
				fields = append(fields, record.KV{Key: m, Val: w})
			} else if v, ok := r.Old.Get(m); ok {
				fields = append(fields, record.KV{Key: m, Val: v.Out()})
			}
		}
		entries = append(entries, record.BlockMap(fields...))
	}
	return record.List(entries...)
}

// writeFile replaces a file in one step, so a failure can't leave half of it.
func writeFile(path string, data []byte) error {
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp") // hidden, so a crash can't leave a stray record
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
