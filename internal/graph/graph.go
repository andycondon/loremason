// Package graph is the context graph as loaded from a workspace's graph/
// folder: the files, the nodes they hold, and the edges between nodes.
package graph

import (
	"slices"
	"strings"

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

// File is one record file. Its record is a Node even when the file can't be
// read, under the ID its filename gives, so links to it still resolve.
type File struct {
	Path   string         // relative to the workspace, with forward slashes
	Type   *registry.Type // from the ID's prefix when that's known, or else the folder
	Folder *registry.Type // the type its folder holds
	Node   *Node
	Record *record.Record // nil when the file couldn't be read or parsed

	rejected map[int]bool // lines the reader already reported
}

// Node is a record, or a sub-record kept in a list inside one.
type Node struct {
	ID       string // bel_7k3m9q, or bel_7k3m9q/s1 for a sub-record
	Type     string // a record type, or a sub-record type such as support_link
	File     *File
	Fields   record.Map // the header, or a sub-record's entry
	Parent   *Node      // the record holding a sub-record; nil for records
	List     string     // for a sub-record, the field holding it, such as support
	Local    string     // a sub-record's own id, such as s1
	Children []*Node    // a record's sub-records, in file order
}

// Edge is one link, from the node whose field holds it.
type Edge struct {
	From *Node
	Name string // the field, such as relies_on, or via for a pin's evidence
	Def  registry.Edge
	To   Ref
	Pin  *Pin // set when the link records the target's revision
	At   Loc
}

// Pin is what a pinned link records about its target.
type Pin struct {
	Rev int
	FP  string
	Via *Edge // to the evidence for the last re-check, if there is one
}

// Loc is where something is written in a file.
type Loc struct {
	Line  int
	Field record.Path
}

// Node returns the node with an ID, or nil.
func (g *Graph) Node(id string) *Node { return g.nodes[id] }

// Resolve returns the node a reference names, or nil. The name is tried as
// an ID or alias first, then as parent/local for a sub-record, since an
// alias may itself contain a slash.
func (g *Graph) Resolve(r Ref) *Node {
	if n := g.byName(r.Name); n != nil {
		return n
	}
	if parent := g.Parent(r); parent != nil {
		return parent.Child(r.Name[strings.LastIndexByte(r.Name, '/')+1:])
	}
	return nil
}

// Parent returns the record that a parent/local reference points inside,
// when the parent part resolves, whether or not the sub-record exists.
func (g *Graph) Parent(r Ref) *Node {
	i := strings.LastIndexByte(r.Name, '/')
	if i < 0 {
		return nil
	}
	if n := g.byName(r.Name[:i]); n != nil && !n.IsSubrecord() {
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

// Header returns the file's fields; none when it couldn't be read.
func (f *File) Header() record.Map {
	if f.Record == nil {
		return record.Map{}
	}
	return f.Record.Header
}

// Misplaced reports whether the ID's prefix names a type other than the
// folder's. The file is then checked against neither type's schema.
func (f *File) Misplaced() bool { return f.Type != f.Folder }

// Rejected reports whether the reader already reported a problem on a line.
func (f *File) Rejected(line int) bool { return f.rejected[line] }

// Identity returns what makes two records of this type look like the same
// thing: the type's identity fields, ignoring case and spacing. ok is false
// when the type has none, or one of them is empty.
func (f *File) Identity() (key string, ok bool) {
	if len(f.Type.Identity) == 0 {
		return "", false
	}
	parts := []string{f.Type.Name}
	for _, field := range f.Type.Identity {
		v := strings.ToLower(strings.Join(strings.Fields(f.Header().Text(field)), " "))
		if v == "" {
			return "", false
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, "\x00"), true
}
