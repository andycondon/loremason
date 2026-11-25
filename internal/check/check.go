// Package check validates a workspace's graph. It only reads.
package check

import (
	"fmt"
	"strings"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/record"
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
		c.path(f)
		c.shape(f)
	}
	for _, e := range g.Edges {
		c.target(e)
	}
	c.duplicates()
	c.ds.Sort()
	return &Result{Records: len(g.Files), Diagnostics: c.ds}, nil
}

// path checks that the filename, its ID prefix, the folder and the id field agree.
func (c *checker) path(f *graph.File) {
	rid := f.Node.ID
	if prefix, ok := id.Prefix(rid); !ok {
		c.ds.Errorf(f.Path, 1, "the filename must be an ID such as %s_7k3m9q.md", f.Folder.Prefix)
	} else if t := c.reg.ByPrefix(prefix); t == nil {
		c.ds.Errorf(f.Path, 1, "no record type has the prefix `%s_`; records in this folder start with `%s_`", prefix, f.Folder.Prefix)
	} else if t != f.Folder {
		c.ds.Errorf(f.Path, 1, "`%s_` is the prefix of type %s, so this file belongs in graph/%s/", prefix, t.Name, t.Folder())
	}
	if f.Record == nil {
		return
	}
	if got := record.Text(f.Record.Header, "id"); got != "" && got != rid {
		c.ds.Errorf(f.Path, f.Record.Line("id"), "the id `%s` doesn't match the filename `%s.md`", got, rid)
	}
}

// target checks that a link points at something, of a type its edge allows.
func (c *checker) target(e *graph.Edge) {
	path := e.From.File.Path
	n := c.g.Resolve(e.To)
	if n == nil {
		c.ds.Errorf(path, e.At.Line, "`%s` points at `%s`, which doesn't exist%s", e.At.Field, e.To, c.missingHint(e.To))
		return
	}
	if !e.Def.Allows(n.Type) {
		c.ds.Errorf(path, e.At.Line, "`%s` points at `%s`, which is of type %s; it must point at %s", e.At.Field, e.To, n.Type, strings.Join(e.Def.To, " or "))
	}
}

// missingHint says what a reference that resolves to nothing might have meant.
func (c *checker) missingHint(r graph.Ref) string {
	parent := c.g.Parent(r)
	if parent == nil {
		return suggest.Hint(suggest.Closest(r.Name, c.g.Names(), 2))
	}
	if locals := parent.Locals(); len(locals) > 0 {
		return fmt.Sprintf("; `%s` has %s", parent.ID, strings.Join(locals, ", "))
	}
	return fmt.Sprintf("; `%s` has no entries to point at", parent.ID)
}

// duplicates warns about records whose identity fields match an earlier one's.
func (c *checker) duplicates() {
	seen := map[string]*graph.File{}
	for _, f := range c.g.Files {
		if f.Record == nil || len(f.Type.Identity) == 0 {
			continue
		}
		key := []string{f.Type.Name}
		for _, field := range f.Type.Identity {
			s := strings.ToLower(strings.Join(strings.Fields(record.Text(f.Record.Header, field)), " "))
			if s == "" {
				key = nil // a field is empty, so there's nothing to compare
				break
			}
			key = append(key, s)
		}
		if key == nil {
			continue
		}
		k := strings.Join(key, "\x00")
		if first := seen[k]; first != nil {
			c.ds.Warnf(f.Path, f.Record.Line(f.Type.Identity[0]), "same %s as %s; if they're the same thing, use that record instead", strings.Join(f.Type.Identity, " and "), first.Node.ID)
		} else {
			seen[k] = f
		}
	}
}
