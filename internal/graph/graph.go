// Package graph is the context graph as loaded from a workspace's graph/
// folder: the files, the nodes they hold, and the edges between nodes.
package graph

import (
	"fmt"
	"slices"
	"strings"

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
}

// Edge is one link, from the node whose field holds it.
type Edge struct {
	From   *Node
	Name   string // the field, such as relies_on, or via for a pin's evidence
	Def    registry.Edge
	To     Ref
	Target *Node // what To resolves to; nil when it doesn't
	Pin    *Pin  // set when the link records the target's revision
	At     Loc
	Item   record.Value // the link as written: a reference, or a {to: …} map
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

// Sealed reports whether something pins the revision. A sealed revision's
// meaning can't change without a new revision.
func (n *Node) Sealed(rev int) bool {
	for _, e := range n.Incoming {
		if e.Pin != nil && e.Pin.Rev == rev {
			return true
		}
	}
	return false
}

// PinnedBy lists the IDs of the records whose links pin the revision.
func (n *Node) PinnedBy(rev int) []string {
	var ids []string
	for _, e := range n.Incoming {
		if e.Pin != nil && e.Pin.Rev == rev && !slices.Contains(ids, e.From.Record().ID) {
			ids = append(ids, e.From.Record().ID)
		}
	}
	return ids
}

// Record returns the record a node belongs to: itself, or its parent.
func (n *Node) Record() *Node {
	if n.Parent != nil {
		return n.Parent
	}
	return n
}

// Stale reports whether a pinned link no longer matches its target: the
// target is at another revision, or its meaning changed since it was pinned.
func (e *Edge) Stale() bool {
	if e.Pin == nil || e.Target == nil || e.Target.Current() == nil {
		return false
	}
	return e.Pin.Rev != e.Target.Current().Rev || e.Pin.FP != e.Target.Fingerprint()
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
