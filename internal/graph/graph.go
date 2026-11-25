// Package graph is the context graph as loaded from a workspace's graph/
// folder: files, the nodes they hold, and the edges between them.
package graph

import (
	"slices"

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

// File is one record file.
type File struct {
	Path   string         // relative to the workspace, with forward slashes
	Type   *registry.Type // from the ID's prefix when that's known, or else the folder
	Folder *registry.Type // the type its folder holds
	Node   *Node          // the record, under the ID its filename gives
	Record *record.Record // nil when the file couldn't be read or parsed
	Bad    map[int]bool   // lines where the reader already reported a problem
}

// Node is a record, or a sub-record kept in a list inside one.
type Node struct {
	ID       string // bel_7k3m9q, or bel_7k3m9q/s1 for a sub-record
	Type     string // a record type, or a sub-record type such as support_link
	File     *File
	Parent   *Node   // the record holding a sub-record; nil for records
	Local    string  // a sub-record's own id, such as s1
	Children []*Node // a record's sub-records, in file order
	Line     int     // where it starts
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
	Via *Edge // the evidence for the last re-check, if any
}

// Loc is where a link is written: its line and its field path, such as
// relies_on[1].to.
type Loc struct {
	Line  int
	Field string
}

// Node returns the node with an ID, or nil.
func (g *Graph) Node(id string) *Node { return g.nodes[id] }

// Resolve returns the node a reference names, or nil. The name is tried as an
// ID or alias first, then as parent/local for a sub-record.
func (g *Graph) Resolve(r Ref) *Node {
	if n := g.lookup(r.Name); n != nil {
		return n
	}
	if parent := g.Parent(r); parent != nil {
		_, local, _ := cutLast(r.Name, "/")
		return parent.Child(local)
	}
	return nil
}

// Parent returns the record a parent/local reference points inside, when
// the parent part resolves, whether or not the sub-record exists.
func (g *Graph) Parent(r Ref) *Node {
	parent, _, found := cutLast(r.Name, "/")
	if !found {
		return nil
	}
	if n := g.lookup(parent); n != nil && n.Parent == nil {
		return n
	}
	return nil
}

func (g *Graph) lookup(name string) *Node {
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

// Record returns the record a node belongs to: itself, or its parent.
func (n *Node) Record() *Node {
	if n.Parent != nil {
		return n.Parent
	}
	return n
}
