// Package graph is the context graph as loaded from a workspace's graph/
// folder: the files, the nodes they hold, and the edges between nodes.
package graph

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
)

// Graph is every record in a workspace, and every link between them.
type Graph struct {
	Files []*File
	Edges []*Edge // in file order, then field order

	nodes   map[string]*Node // by ID; sub-records as parent/local
	aliases map[string]*Node
}

// File is where a record is written, and how reading it went. The record
// itself is its Node, which exists even when the file can't be read, under
// the ID its filename gives, so links to it still resolve.
type File struct {
	Path   string         // relative to the workspace, with forward slashes
	Type   *registry.Type // from the ID's prefix when that's known, or else the folder
	Folder *registry.Type // the type its folder holds
	Node   *Node
	Record *record.Record // nil when the file couldn't be read or parsed
	Sum    [32]byte       // SHA-256 of the file as it was read, so a writer can tell it hasn't changed since

	rejected map[int]bool // lines the reader already reported
}

// Node is a record, or a sub-record kept in a list inside one.
type Node struct {
	ID       string         // bel_7k3m9q, or bel_7k3m9q/s1 for a sub-record
	Kind     *registry.Kind // its record type, or its kind of sub-record
	File     *File
	Fields   record.Map // the header, or a sub-record's entry; none when the file couldn't be read
	Parent   *Node      // the record holding a sub-record; nil for records
	Local    string     // a sub-record's own id, such as s1
	Children []*Node    // a record's sub-records, in file order

	// Revisions are a revisable record's revisions, oldest first; the last
	// is the current one. A revisable record whose file has no revisions
	// list yet is at revision 1, which is Implicit until sync writes it.
	Revisions []Revision
	// Incoming are the edges that point at this node.
	Incoming []*Edge
	// Outgoing are the edges whose link this node holds.
	Outgoing []*Edge
}

// Edge is one link, from the node whose field holds it.
type Edge struct {
	From   *Node
	Name   string // the field, such as relies_on, or via for a pin's evidence
	Def    registry.Edge
	To     Ref
	Target *Node // what To resolves to; nil when it doesn't
	Pin    *Pin  // set when the link records the target's revision
	// Via is the link in the pin's via, to the evidence for its last re-check;
	// nil when the pin has none.
	Via  *Edge
	At   Loc
	Item record.Value // the link as written: a reference, or a {to: …} map
}

// Pinnable reports whether sync should pin the link: the edge should pin
// its target (registry.Edge.ShouldPin), and the link is in a record's own
// fields, not a sub-record's.
func (e *Edge) Pinnable() bool {
	return e.Target != nil && !e.From.IsSubrecord() && e.Def.ShouldPin(e.Target.Kind)
}

// Loc is where a link is written: the file and line, and the field.
type Loc struct {
	diag.Pos
	Field record.Path
}

// Node returns the node with an ID, or nil. It doesn't resolve aliases;
// Resolve does.
func (g *Graph) Node(id string) *Node { return g.nodes[id] }

// Lookup parses s as a reference, such as bel_7k3m9q, an alias or
// bel_7k3m9q/s1, and returns the node it names. It's for references that
// arrive as text, such as a command's argument; the error says whether s
// isn't a reference or names no record.
func (g *Graph) Lookup(s string) (*Node, error) {
	r, err := ParseRef(s)
	if err != nil {
		return nil, err
	}
	if n := g.Resolve(r); n != nil {
		return n, nil
	}
	return nil, fmt.Errorf("no record is called `%s`", s)
}

// Resolve returns the node a reference names, or nil. The name is tried as
// an ID or alias first, then as parent/local for a sub-record, since an
// alias may itself contain a slash.
func (g *Graph) Resolve(r Ref) *Node {
	if n := g.byName(r.Name); n != nil {
		return n
	}
	if parent := g.Parent(r); parent != nil {
		_, local, _ := splitSubrecord(r.Name)
		return parent.Child(local)
	}
	return nil
}

// Parent returns the record that a parent/local reference points inside,
// when the parent part resolves, whether or not the sub-record exists.
func (g *Graph) Parent(r Ref) *Node {
	name, _, ok := splitSubrecord(r.Name)
	if !ok {
		return nil
	}
	if n := g.byName(name); n != nil && !n.IsSubrecord() {
		return n
	}
	return nil
}

func (g *Graph) byName(name string) *Node {
	if n := g.nodes[name]; n != nil {
		return n
	}
	return g.aliases[name]
}

// Names lists every ID and alias, sorted.
func (g *Graph) Names() []string {
	names := make([]string, 0, len(g.nodes)+len(g.aliases))
	for id := range g.nodes {
		names = append(names, id)
	}
	for a := range g.aliases {
		names = append(names, a)
	}
	slices.Sort(names)
	return names
}

// IsSubrecord reports whether the node lives inside another record.
func (n *Node) IsSubrecord() bool { return n.Parent != nil }

// Revisable reports whether the node is a record that keeps revisions.
func (n *Node) Revisable() bool { return n.Kind.Revisable && !n.IsSubrecord() }

// Current returns the current revision. A revisable record always has one
// (perhaps implicit); for any other node it's nil.
func (n *Node) Current() *Revision {
	if len(n.Revisions) == 0 {
		return nil
	}
	return &n.Revisions[len(n.Revisions)-1]
}

// Revision returns a revision by number, or nil.
func (n *Node) Revision(rev int) *Revision {
	for i := range n.Revisions {
		if n.Revisions[i].Rev == rev {
			return &n.Revisions[i]
		}
	}
	return nil
}

// Seals reports whether the link seals the revision it pins: it is pinned and
// its edge propagates. A quote is pinned but doesn't propagate, so it records
// a revision without freezing it.
func (e *Edge) Seals() bool { return e.Pin != nil && e.Def.Propagates }

// SealedBy says why a revision is sealed: the IDs of the records whose
// propagating links pin it, then, for the current revision of a record (not
// a sub-record), the reasons the type's sealed key gives ("accepted",
// "settled", "its plan plan_x is accepted"). Those apply only once the
// revision's fingerprint is recorded: a revision `mason revise` just opened
// has none, so it can be edited until `mason sync` records it. It's nil when
// nothing seals the revision. A sealed revision's meaning can't change
// without a new revision.
func (n *Node) SealedBy(rev int) []string {
	var reasons []string
	for _, e := range n.Incoming {
		if e.Seals() && e.Pin.Rev == rev && !slices.Contains(reasons, e.From.Record().ID) {
			reasons = append(reasons, e.From.Record().ID)
		}
	}
	if cur := n.Current(); cur != nil && cur.Rev == rev && !n.IsSubrecord() && !cur.Implicit && !cur.FP.IsZero() {
		reasons = append(reasons, n.StateSeals()...)
	}
	return reasons
}

// Quoted reports whether a link that doesn't seal, a quote, pins the
// revision. A quote doesn't seal a revision, but it lets it be revised, so
// the quoted wording is kept.
func (n *Node) Quoted(rev int) bool {
	for _, e := range n.Incoming {
		if e.Pin != nil && !e.Def.Propagates && e.Pin.Rev == rev {
			return true
		}
	}
	return false
}

// StatusHolder returns the node whose status decides the record's state: the
// record itself, or, for a record with no status of its own whose type has a
// status_from edge, the record that edge leads to (the first it resolves,
// followed further if that has none either). It is nil when the edges lead
// round a cycle.
func (n *Node) StatusHolder() *Node {
	rec := n.Record()
	seen := map[*Node]bool{}
	for !seen[rec] {
		seen[rec] = true
		from := rec.Type().StatusFrom
		if rec.Status() != "" || from == "" {
			return rec
		}
		next := rec
		for _, e := range rec.Outgoing {
			if e.Name == from && e.Target != nil {
				next = e.Target.Record()
				break
			}
		}
		if next == rec {
			return rec
		}
		rec = next
	}
	return nil
}

// StateSeals gives the reasons the type's sealed key seals the node as it is
// now, whether or not its revision's fingerprint has been recorded; SealedBy
// applies them only once it has.
func (n *Node) StateSeals() []string {
	s := n.Type().Sealed
	var reasons []string
	if h := n.StatusHolder(); h != nil && slices.Contains(s.Status, h.Status()) {
		if h == n.Record() {
			reasons = append(reasons, h.Status())
		} else {
			reasons = append(reasons, fmt.Sprintf("its %s %s is %s", h.Type().Name, h.ID, h.Status()))
		}
	}
	if s.Settled && n.SettledByAnswer() {
		reasons = append(reasons, "settled")
	}
	return reasons
}

// SealedByStatus reports whether the status of the record, or of the one it
// takes its status from, is one its type's sealed key lists, as an accepted
// decision's is. Unlike StateSeals, it ignores settling and whether the
// fingerprint is recorded.
func (n *Node) SealedByStatus() bool {
	h := n.StatusHolder()
	return h != nil && slices.Contains(n.Type().Sealed.Status, h.Status())
}

// Sealed reports whether anything seals the revision: see SealedBy.
func (n *Node) Sealed(rev int) bool { return len(n.SealedBy(rev)) > 0 }

// Type returns the record type of the node's record.
func (n *Node) Type() *registry.Type { return n.Record().File.Type }

// Layer returns the layer of the node's record: knowing, committing or scope.
func (n *Node) Layer() string { return n.Type().Layer }

// Status returns the header's status, or "" when there isn't one.
func (n *Node) Status() string { return n.Fields.Text("status") }

// Record returns the record a node belongs to: itself, or its parent.
func (n *Node) Record() *Node {
	if n.Parent != nil {
		return n.Parent
	}
	return n
}

// Concern is a reason a link needs a re-check, as bit flags: a link can have
// several.
type Concern uint8

const (
	// Stale: the pinned target is at another revision, or its meaning changed
	// since it was pinned.
	Stale Concern = 1 << iota
	// Reopens: the target has fallen, so the commitment resting on it reopens.
	Reopens
	// Orphaned: the target is no longer in force without having fallen, such
	// as a withdrawn belief or a superseded task.
	Orphaned
	// Doubted: the target is a belief whose support is undercut or whose
	// evidence is undermined, and no re-check since says otherwise.
	Doubted
)

// concerns is every concern, in the order String lists them.
var concerns = []Concern{Stale, Reopens, Orphaned, Doubted}

// Each returns the concerns c includes, one flag apiece, in the order String
// lists them.
func (c Concern) Each() []Concern {
	var out []Concern
	for _, f := range concerns {
		if c.Has(f) {
			out = append(out, f)
		}
	}
	return out
}

// Has reports whether c includes every concern in o.
func (c Concern) Has(o Concern) bool { return c&o == o }

// String gives the reason in words, a comma between several. It is "" for none.
func (c Concern) String() string {
	var parts []string
	for _, f := range c.Each() {
		switch f {
		case Stale:
			parts = append(parts, "stale")
		case Reopens:
			parts = append(parts, "reopens")
		case Orphaned:
			parts = append(parts, "rests on a record no longer in force")
		case Doubted:
			parts = append(parts, "rests on a belief whose support is undercut or undermined")
		}
	}
	return strings.Join(parts, ", ")
}

// Concerns says why the link needs a re-check, or 0 when it doesn't. Only a
// link that propagates to a target, from a record in the committing layer that
// is still in force (not closed or fallen, as a rejected or superseded one
// is), is ever a concern: every source of a propagating edge is committing.
func (e *Edge) Concerns() Concern {
	if !e.Def.Propagates || e.Target == nil || e.From.Layer() != registry.Committing || !e.From.InForce() {
		return 0
	}
	var c Concern
	if e.Pin != nil && e.Target.Current() != nil &&
		(e.Pin.Rev != e.Target.Current().Rev || e.Pin.FP != e.Target.Fingerprint()) {
		c |= Stale
	}
	switch {
	case e.Target.Type().IsFallen(e.Target.Status()):
		c |= Reopens
	case !e.Target.InForce():
		c |= Orphaned
	}
	if e.doubts() {
		c |= Doubted
	}
	return c
}

// Stale reports whether a pinned link no longer matches its target: the
// target is at another revision, or its meaning changed since it was pinned.
// A pin that doesn't seal, such as a quote's, is never stale.
func (e *Edge) Stale() bool { return e.Concerns().Has(Stale) }

// Reopens reports whether the link reopens its source: its target's status is
// one that the target's type lists as fallen. A target that no longer holds
// for another reason, such as a withdrawn belief, reopens nothing.
func (e *Edge) Reopens() bool { return e.Concerns().Has(Reopens) }

// Orphaned reports whether the link rests on a record that is no longer in
// force, such as a withdrawn belief, a superseded task, or a step of a
// superseded plan. A target that has fallen reopens its source instead (see
// Reopens), so it isn't reported here too. A withdrawn belief doesn't reopen
// what relied on it, but whoever relied on it should look again.
func (e *Edge) Orphaned() bool { return e.Concerns().Has(Orphaned) }

// Doubts reports whether the link relies on a belief that is doubted and no
// re-check has cleared it: see doubts.
func (e *Edge) Doubts() bool { return e.Concerns().Has(Doubted) }

// InForce reports whether the node's record is still in force: its status
// holder's status (see StatusHolder) is neither one its type lists as closed
// nor as fallen. A type with no such statuses is always in force.
func (n *Node) InForce() bool {
	h := n.StatusHolder()
	if h == nil {
		return true
	}
	t := h.Type()
	return !t.IsClosed(h.Status()) && !t.IsFallen(h.Status())
}

// DoubtedBy returns the evidence that makes a belief doubted, sorted by ID:
// evidence that `undercuts` one of the belief's support links, or that
// `undermines` evidence a support link cites. Only a link that supports the
// belief counts; evidence undercutting a link that disputes it says nothing
// against the belief. A belief that has fallen or is closed is settled, not
// doubted, so it has none. The rule is for beliefs; see 01-model.md, section
// 3.2.
func (n *Node) DoubtedBy() []*Node {
	if !n.HasSupport() || !n.InForce() {
		return nil
	}
	var by []*Node
	add := func(ev *Node) {
		if !slices.Contains(by, ev) {
			by = append(by, ev)
		}
	}
	for _, link := range n.SupportLinks(Supports) {
		for _, in := range link.Incoming {
			if in.Name == EdgeUndercuts {
				add(in.From)
			}
		}
		for _, out := range link.Outgoing {
			if out.Name != EdgeEvidence || out.Target == nil {
				continue
			}
			for _, in := range out.Target.Incoming {
				if in.Name == EdgeUndermines {
					add(in.From)
				}
			}
		}
	}
	slices.SortFunc(by, func(a, b *Node) int { return strings.Compare(a.ID, b.ID) })
	return by
}

// Doubted reports whether a belief that still stands has lost some of its
// footing: see DoubtedBy. It stays so until the link is removed, the belief's
// status changes, or a link that relies on it is re-checked (see Doubts).
func (n *Node) Doubted() bool { return len(n.DoubtedBy()) > 0 }

// ObservedAt parses the node's observed_at, a date or a time as
// common.json's time allows: a date alone is midnight UTC, and a time with
// no offset is UTC. It is false when there is none or it can't be parsed.
func (n *Node) ObservedAt() (time.Time, bool) {
	v := strings.TrimSpace(n.Fields.Text("observed_at"))
	if len(v) > 10 {
		v = v[:10] + "T" + strings.Join(strings.Fields(v[11:]), "") // the separator may be a t or a space
	}
	v = strings.Replace(v, "z", "Z", 1)
	for _, layout := range []string{
		time.DateOnly,
		"2006-01-02T15:04:05.999999999Z07:00", "2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04Z07:00", "2006-01-02T15:04",
	} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ObservedNotAfter reports whether evidence was observed no later than
// other, comparing the instants of their observed_at. Equal times count, so
// a tie clears a doubt. Either one's observed_at missing or unparsable is
// false: such evidence clears nothing.
func (n *Node) ObservedNotAfter(other *Node) bool {
	a, ok := n.ObservedAt()
	b, ok2 := other.ObservedAt()
	return ok && ok2 && !a.After(b)
}

// doubts is the doubted part of Concerns: the target is doubted, and no
// re-check clears it. A re-check clears it when the link's pin names, in via,
// evidence observed on or after all of the evidence that makes the belief
// doubted (see ObservedAt). A newer undermining or undercutting doubts it
// again.
func (e *Edge) doubts() bool {
	by := e.Target.DoubtedBy()
	if len(by) == 0 {
		return false
	}
	if e.Via == nil || e.Via.Target == nil {
		return true
	}
	for _, ev := range by {
		if !ev.ObservedNotAfter(e.Via.Target) {
			return true
		}
	}
	return false
}

// Settles reports whether the link settles its target: its edge lists the
// source's status as one that settles, such as an accepted decision
// answering a question.
func (e *Edge) Settles() bool { return slices.Contains(e.Def.Settles, e.From.Status()) }

// Pending reports whether the link is an answer still waiting: its edge
// lists the source's status as pending, such as a proposed decision
// answering a question. A dropped target waits for no answer.
func (e *Edge) Pending() bool {
	return slices.Contains(e.Def.Pending, e.From.Status()) && (e.Target == nil || !e.Target.Dropped())
}

// Attr returns a text field of a link written as a map, such as role on
// involves, or "" when the link is a plain reference or has no such field.
func (e *Edge) Attr(name string) string {
	m, ok := e.Item.Map()
	if !ok {
		return ""
	}
	return m.Text(name)
}

// Progress returns the node's progress: the header's, or else the default its
// type's schema declares (todo for a task). It's "" for a sub-record, which
// gets no default, and for a node whose type has no progress field.
func (n *Node) Progress() string {
	if p := n.Fields.Text("progress"); p != "" {
		return p
	}
	if n.IsSubrecord() {
		return ""
	}
	return n.Type().Default("progress")
}

// Dropped reports whether the node has a dropped field, the reason a question
// was given up on.
func (n *Node) Dropped() bool {
	_, dropped := n.Fields.Get("dropped")
	return dropped
}

// SettledByAnswer reports whether a link to the node settles it, such as an
// accepted decision answering a question. A dropped node isn't settled by an
// answer, though Settled counts it.
func (n *Node) SettledByAnswer() bool {
	for _, e := range n.Incoming {
		if e.Settles() {
			return true
		}
	}
	return false
}

// Settled reports whether the node needs no more answering: it is dropped (it
// has a dropped field), or a link to it settles it (SettledByAnswer). It is
// asked of any node.
func (n *Node) Settled() bool { return n.Dropped() || n.SettledByAnswer() }

// Open reports whether the node still wants answering: its type can be
// settled (a question), and it is not settled, or a link to it is an answer
// still waiting (Pending, which a dropped node has none of).
func (n *Node) Open() bool {
	if n.IsSubrecord() || !n.Type().Settleable {
		return false
	}
	if !n.Settled() {
		return true
	}
	for _, e := range n.Incoming {
		if e.Pending() {
			return true
		}
	}
	return false
}

// Members returns the records that belong to an initiative, sorted by ID:
// those whose part_of links lead to it, directly or through other records
// (a step through its plan, a requirement through its PRD), and their
// sub-records. The initiative itself isn't one. A cycle of part_of links
// ends.
func (g *Graph) Members(ini *Node) []*Node {
	seen := map[*Node]bool{ini: true}
	var out []*Node
	queue := []*Node{ini}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, e := range n.Incoming {
			if e.Name != "part_of" || e.From.IsSubrecord() || seen[e.From] {
				continue
			}
			seen[e.From] = true
			out = append(out, e.From)
			out = append(out, e.From.Children...)
			queue = append(queue, e.From)
		}
	}
	slices.SortFunc(out, func(a, b *Node) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Produced returns the records made in the initiative's activities: those
// with a generated_by link to an activity that is one of its members, sorted
// by ID. They aren't members, because what a session finds doesn't belong to
// the initiative. A record that is already a member is left out.
func (g *Graph) Produced(ini *Node) []*Node {
	members := g.Members(ini)
	in := make(map[*Node]bool, len(members))
	for _, m := range members {
		in[m] = true
	}
	seen := map[*Node]bool{}
	var out []*Node
	for _, m := range members {
		if m.Kind.Name != "activity" {
			continue
		}
		for _, e := range m.Incoming {
			if e.Name != "generated_by" || in[e.From] || seen[e.From] {
				continue
			}
			seen[e.From] = true
			out = append(out, e.From)
		}
	}
	slices.SortFunc(out, func(a, b *Node) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Behind returns the target's revisions after the one the link pins.
func (e *Edge) Behind() []Revision {
	if e.Pin == nil || e.Target == nil {
		return nil
	}
	var revs []Revision
	for _, r := range e.Target.Revisions {
		if r.Rev > e.Pin.Rev {
			revs = append(revs, r)
		}
	}
	return revs
}

// Child returns a sub-record by its local id, or nil.
func (n *Node) Child(local string) *Node {
	for _, c := range n.Children {
		if c.Local == local {
			return c
		}
	}
	return nil
}

// Locals lists the local ids of a record's sub-records.
func (n *Node) Locals() []string {
	var ids []string
	for _, c := range n.Children {
		ids = append(ids, c.Local)
	}
	return ids
}

// At returns the position of a line in the file.
func (f *File) At(line int) diag.Pos { return diag.Pos{Path: f.Path, Line: line} }

// Misplaced reports whether the ID's prefix names a type other than the
// folder's. The file is then checked against neither type's schema.
func (f *File) Misplaced() bool { return f.Type != f.Folder }

// Rejected reports whether reading the file already reported an error at
// line, so a check doesn't report it twice.
func (f *File) Rejected(line int) bool { return f.rejected[line] }

// Identity is what makes two records look like the same thing, such as two
// artifacts with the same url.
type Identity struct {
	Key    string   // the kind, then each identity field ignoring case and spacing
	Fields []string // the kind's identity fields
	At     diag.Pos // where the first of them is written
}

// Identity returns the node's identity. ok is false when its kind has no
// identity fields, or one of them is empty.
func (n *Node) Identity() (id Identity, ok bool) {
	fields := n.Kind.Identity
	if len(fields) == 0 {
		return Identity{}, false
	}
	parts := []string{n.Kind.Name}
	for _, field := range fields {
		v := strings.ToLower(strings.Join(strings.Fields(n.Fields.Text(field)), " "))
		if v == "" {
			return Identity{}, false
		}
		parts = append(parts, v)
	}
	return Identity{Key: strings.Join(parts, "\x00"), Fields: fields, At: n.File.At(n.Fields.KeyLine(fields[0]))}, true
}
