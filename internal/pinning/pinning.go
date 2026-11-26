// Package pinning keeps links pinned to what they were built against. It
// works in two steps: Sync, Revise and Recheck decide what would change and
// say why, and Apply writes it. The forms of a revision entry and a pinned
// link belong to package graph; this package only decides what they should
// say. The design is in docs/context-graph/03-implementation.md, sections 4
// and 5.
package pinning

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

// Plan is what sync, revise or a re-check would do.
type Plan struct {
	Actions  []Action
	Warnings []Action // things done that a person should know about
	Skipped  []string // files left alone because they have errors

	edits map[*graph.File]*fileEdit
}

// Broken is the set of files, by path, that have errors. Tools that write
// leave them alone, so re-writing a field they can't read can't lose what a
// person wrote.
type Broken map[string]bool

// fileEdit is the new state of one file's tool-owned fields.
type fileEdit struct {
	file      *graph.File
	revisions []graph.Revision          // the whole list, when it changes
	pins      map[*graph.Edge]graph.Pin // links to write pinned, by the edge read from the file
}

// Empty reports whether the plan changes nothing.
func (p *Plan) Empty() bool { return len(p.Actions) == 0 }

func newPlan() *Plan { return &Plan{edits: map[*graph.File]*fileEdit{}} }

func (p *Plan) edit(f *graph.File) *fileEdit {
	e := p.edits[f]
	if e == nil {
		e = &fileEdit{file: f, pins: map[*graph.Edge]graph.Pin{}}
		p.edits[f] = e
	}
	return e
}

func (p *Plan) say(f *graph.File, format string, args ...any) {
	p.Actions = append(p.Actions, Action{f.Path, fmt.Sprintf(format, args...)})
}

func (p *Plan) warn(f *graph.File, format string, args ...any) {
	p.Warnings = append(p.Warnings, Action{f.Path, fmt.Sprintf(format, args...)})
}

// Sync plans what `mason sync` does, in an order that matters, because a pin
// seals the revision it names:
//
//  1. write revision 1 for each revisable record that has no revisions yet;
//  2. record the fingerprint of each current revision nothing pins yet, so
//     drafts can change freely;
//  3. pin each bare link that should be pinned, to the revision it names or
//     else the current one;
//  4. move a pin forward when every later revision of its target is
//     non-breaking. A link pinned in step 3 gets this too, so a second sync
//     has nothing left to do.
//
// Files with errors are skipped, and so are links to them.
func Sync(g *graph.Graph, broken Broken, today string) *Plan {
	p := newPlan()
	revs := map[*graph.Node][]graph.Revision{} // each record's revisions after steps 1 and 2
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
		if e.Pinnable() && !broken[e.From.File.Path] && !broken[e.Target.File.Path] {
			p.pin(e, revs[e.Target])
		}
	}
	return p
}

// recordRevision does steps 1 and 2 for one record, and returns its
// revisions as they will be.
func (p *Plan) recordRevision(f *graph.File, today string) []graph.Revision {
	n := f.Node
	rs := slices.Clone(n.Revisions)
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
func (p *Plan) pin(e *graph.Edge, target []graph.Revision) {
	if len(target) == 0 {
		return
	}
	f, at, cur := e.From.File, e.Item.Path, target[len(target)-1]
	if e.Pin == nil {
		// A bare link pins what it names: a revision written as @rN, or
		// else the current one.
		r := cur
		if e.To.Rev > 0 {
			i := slices.IndexFunc(target, func(r graph.Revision) bool { return r.Rev == e.To.Rev })
			if i < 0 {
				return // check reports a revision that doesn't exist
			}
			r = target[i]
		}
		why := ""
		if r.Rev < cur.Rev && allNonBreaking(target, r.Rev) {
			why = fmt.Sprintf(", since every revision after r%d is non-breaking", r.Rev)
			r = cur
		}
		p.edit(f).pins[e] = graph.Pin{Rev: r.Rev, FP: r.FP}
		p.say(f, "pin `%s` to %s@r%d%s", at, e.Target.ID, r.Rev, why)
		return
	}
	if e.Pin.Rev < cur.Rev && allNonBreaking(target, e.Pin.Rev) {
		// The re-check a via recorded was against an older revision, so it's
		// dropped; the revisions' own change values say why the pin moved.
		p.edit(f).pins[e] = graph.Pin{Rev: cur.Rev, FP: cur.FP}
		p.say(f, "re-pin `%s` to %s@r%d: every revision since r%d is non-breaking", at, e.Target.ID, cur.Rev, e.Pin.Rev)
	}
}

func allNonBreaking(revs []graph.Revision, after int) bool {
	for _, r := range revs {
		if r.Rev > after && r.Breaking {
			return false
		}
	}
	return true
}

// Revise plans `mason revise`. It keeps the current revision's wording (or,
// for long records, the commit holding it) and opens a new revision. The
// edit comes after, and the next sync records the new revision's
// fingerprint. commit is the commit holding the file as it is now, or empty
// when there isn't one.
func Revise(n *graph.Node, breaking bool, broken Broken, today, commit string) (*Plan, error) {
	f := n.File
	switch {
	case !n.Revisable():
		return nil, fmt.Errorf("%s is of type %s, which has no revisions", n.ID, n.Kind.Name)
	case broken[f.Path]:
		return nil, fmt.Errorf("%s has errors; fix them first (see `mason validate`)", f.Path)
	}
	rs := slices.Clone(n.Revisions)
	cur := &rs[len(rs)-1]
	fp := n.Fingerprint()
	switch {
	case !n.Sealed(cur.Rev):
		return nil, fmt.Errorf("nothing pins revision %d of %s yet, so it can be edited as it is; no new revision is needed", cur.Rev, n.ID)
	case !cur.FP.IsZero() && cur.FP != fp:
		return nil, fmt.Errorf("the meaning of %s already changed after revision %d was pinned; undo the change first, so the old wording can be kept", n.ID, cur.Rev)
	}
	p := newPlan()
	if cur.Implicit || cur.FP.IsZero() {
		cur.Implicit, cur.FP = false, fp
		if cur.Date == "" {
			cur.Date = today
		}
	}
	switch {
	case n.KeepsWordingInline():
		cur.Kept = n.Wording()
	case commit != "":
		cur.Commit = commit
	default:
		p.warn(f, "no commit holds revision %d's text, since the file has changes not yet committed (or isn't in git), so its old wording won't be recoverable; commit before revising to keep it", cur.Rev)
	}
	rs = append(rs, graph.Revision{Rev: cur.Rev + 1, Date: today, FP: fp, Breaking: breaking})
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
	p.edit(e.From.File).pins[e] = graph.Pin{Rev: cur.Rev, FP: cur.FP, Via: &graph.Ref{Name: evidence.ID}}
	p.say(e.From.File, "re-pin `%s` to %s@r%d: it still holds, as %s says", e.Item.Path, e.Target.ID, cur.Rev, evidence.ID)
	return p, nil
}

// Apply writes a plan's changes under root. It prepares every file first
// and writes none if any can't be prepared, for instance because it changed
// since the plan was made.
func Apply(root string, p *Plan) error {
	type output struct {
		path string
		data []byte
	}
	var outs []output
	for _, f := range slices.SortedFunc(maps.Keys(p.edits), func(a, b *graph.File) int { return strings.Compare(a.Path, b.Path) }) {
		path := filepath.Join(root, filepath.FromSlash(f.Path))
		data, err := p.edits[f].apply(path)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		outs = append(outs, output{path, data})
	}
	for _, o := range outs {
		if err := writeFile(o.path, o.data); err != nil {
			return err
		}
	}
	return nil
}

var errChanged = errors.New("it changed after it was read; run the command again")

// apply returns the file with the edit made.
func (e *fileEdit) apply(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if sha256.Sum256(data) != e.file.Sum {
		return nil, errChanged
	}
	h, err := splice.Edit(data)
	if err != nil {
		return nil, err
	}
	n := e.file.Node
	if e.revisions != nil {
		var entries []record.Out
		for _, r := range e.revisions {
			entries = append(entries, r.Out(n))
		}
		if v, ok := n.Fields.Get("revisions"); ok {
			h.Set("revisions", v.Rewrite(record.List(entries...)))
		} else {
			h.Set("revisions", record.Render("revisions", record.List(entries...)))
		}
	}
	for _, field := range e.pinnedFields() {
		v, _ := n.Fields.Get(field)
		h.Set(field, v.Rewrite(e.links(v)))
	}
	return h.Bytes(), nil
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

// links writes a link field as a list, pinning the links in the plan and
// leaving the rest as they were.
func (e *fileEdit) links(field record.Value) record.Out {
	byPath := map[string]*graph.Edge{}
	for edge := range e.pins {
		byPath[edge.Item.Path.String()] = edge
	}
	var items []record.Out
	for _, item := range field.Items() {
		edge, ok := byPath[item.Path.String()]
		if !ok {
			items = append(items, item.Out())
			continue
		}
		items = append(items, e.pins[edge].Out(edge.To.Name).WithCommentsOf(item))
	}
	return record.List(items...)
}

// writeFile replaces a file in one step, so a failure can't leave half of
// it. A symlink is followed, so the file it points at is the one changed,
// and the file keeps its permissions.
func writeFile(path string, data []byte) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(real), "."+filepath.Base(real)+".tmp") // hidden, so a crash can't leave a stray record
	if err := os.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp, real)
}
