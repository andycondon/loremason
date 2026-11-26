// Package check validates a workspace's graph. It only reads.
//
// Loading the graph reports what keeps files, names and links out of it.
// The rules here then check what's left: that each file is in the right
// place, that each header has the shape its schema gives, that each link
// points at something of a type its edge allows, that revisions and pins
// are consistent and sealed revisions haven't changed, and that no two
// records look like the same thing.
package check

import (
	"fmt"
	"strings"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// Result is what a check found.
type Result struct {
	Records     int
	Diagnostics diag.List
}

type checker struct {
	reg *registry.Registry
	g   *graph.Graph
	ds  diag.List
}

// Run loads the graph at root and checks it.
func Run(root string, reg *registry.Registry) (*Result, error) {
	g, ds, err := graph.Load(root, reg)
	if err != nil {
		return nil, err
	}
	c := &checker{reg: reg, g: g, ds: ds}
	for _, f := range g.Files {
		c.placement(f)
		c.shape(f)
		c.revisionNumbers(f)
		c.sealed(f)
	}
	for _, e := range g.Edges {
		c.target(e)
		c.pinning(e)
	}
	c.duplicates()
	c.ds.Sort()
	return &Result{Records: len(g.Files), Diagnostics: c.ds}, nil
}

// placement checks that the filename is an ID whose prefix matches the
// folder, and that the id field matches the filename.
func (c *checker) placement(f *graph.File) {
	rid := f.Node.ID
	if prefix, ok := id.Prefix(rid); !ok {
		c.ds.Errorf(f.At(1), "the filename must be an ID such as %s_7k3m9q.md", f.Folder.Prefix)
	} else if t := c.reg.ByPrefix(prefix); t == nil {
		c.ds.Errorf(f.At(1), "no record type has the prefix `%s_`; records in this folder start with `%s_`", prefix, f.Folder.Prefix)
	} else if f.Misplaced() {
		c.ds.Errorf(f.At(1), "`%s_` is the prefix of type %s, so this file belongs in graph/%s/", prefix, t.Name, t.Folder())
	}
	if got := f.Node.Fields.Text("id"); got != "" && got != rid {
		c.ds.Errorf(f.At(f.Node.Fields.KeyLine("id")), "the id `%s` doesn't match the filename `%s.md`", got, rid)
	}
}

// target checks that a link points at something, of a type its edge allows.
func (c *checker) target(e *graph.Edge) {
	n := e.Target
	switch {
	case n == nil:
		c.ds.Errorf(e.At.Pos, "`%s` points at `%s`, which doesn't exist%s", e.At.Field, e.To, c.missingHint(e.To))
	case !e.Def.Allows(n.Kind):
		c.ds.Errorf(e.At.Pos, "`%s` points at `%s`, which is of type %s; it must point at %s", e.At.Field, e.To, n.Kind.Name, orList(e.Def.To))
	}
}

// missingHint says what a reference to nothing might have meant: another
// sub-record of the same record, or a name a typo away.
func (c *checker) missingHint(r graph.Ref) string {
	parent := c.g.Parent(r)
	switch {
	case parent == nil:
		return suggest.Hint(suggest.Closest(r.Name, c.g.Names(), 2))
	case len(parent.Children) == 0:
		return fmt.Sprintf("; `%s` has no entries to point at", parent.ID)
	default:
		return fmt.Sprintf("; `%s` has %s", parent.ID, strings.Join(parent.Locals(), ", "))
	}
}

// duplicates warns about a record that looks like the same thing as an
// earlier one.
func (c *checker) duplicates() {
	first := map[string]*graph.File{}
	for _, f := range c.g.Files {
		id, ok := f.Node.Identity()
		if !ok {
			continue
		}
		if earlier := first[id.Key]; earlier != nil {
			c.ds.Warnf(id.At, "same %s as %s; if they're the same thing, use that record instead", andList(id.Fields), earlier.Node.ID)
		} else {
			first[id.Key] = f
		}
	}
}
