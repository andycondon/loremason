package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// links checks that every link points at a record of an allowed type.
// Values of the wrong shape are left to the schema.
func (c *checker) links(f *file) {
	for name, edge := range f.typ.Edges {
		c.follow(f, edge, f.rec.Fields[name], name)
	}
	for field, sub := range f.typ.Subrecords {
		list, _ := f.rec.Fields[field].([]any)
		for i, item := range list {
			entry, _ := item.(map[string]any)
			for name, edge := range sub.Edges {
				c.follow(f, edge, entry[name], field, fmt.Sprint(i), name)
			}
		}
	}
}

// follow checks a link field: an ID, a {to: …} entry, or a list of either.
func (c *checker) follow(f *file, edge registry.Edge, v any, at ...string) {
	switch v := v.(type) {
	case []any:
		for i, item := range v {
			c.follow(f, edge, item, append(slices.Clone(at), fmt.Sprint(i))...)
		}
	case map[string]any:
		if to, ok := v["to"].(string); ok {
			c.follow(f, edge, to, append(slices.Clone(at), "to")...)
		}
		if via, ok := v["via"].(string); ok {
			c.follow(f, registry.Edge{To: []string{"evidence"}}, via, append(slices.Clone(at), "via")...)
		}
	case string:
		if v == "" || strings.ContainsAny(v, " \t\r\n") {
			return
		}
		line, where := f.rec.Line(at...), fieldPath(at)
		typ, hint := c.resolve(v)
		if typ == "" {
			c.errorf(f.path, line, "`%s` points at `%s`, which doesn't exist%s", where, v, hint)
		} else if !edge.Allows(typ) {
			c.errorf(f.path, line, "`%s` points at `%s`, which is of type %s; it must point at %s", where, v, typ, strings.Join(edge.To, " or "))
		}
	}
}

// resolve returns the type of what a reference names. When there's nothing
// there, it returns "" and the rest of the message.
func (c *checker) resolve(ref string) (typ, hint string) {
	ref = id.TrimRevision(ref)
	if t, ok := c.byID[ref]; ok {
		return t.typ, ""
	}
	if f := c.aliases[ref]; f != nil {
		return f.typ.Name, ""
	}
	if parent, local, ok := id.SplitSub(ref); ok {
		f := c.aliases[parent]
		if t, ok := c.byID[parent]; ok && t.in.id == parent {
			f = t.in
		}
		if f != nil {
			if t, ok := c.byID[f.id+"/"+local]; ok {
				return t.typ, ""
			}
			var have []string
			for _, n := range c.names {
				if rest, ok := strings.CutPrefix(n, f.id+"/"); ok {
					have = append(have, rest)
				}
			}
			if len(have) == 0 {
				return "", fmt.Sprintf("; `%s` has no entries to point at", parent)
			}
			return "", fmt.Sprintf("; `%s` has %s", parent, strings.Join(have, ", "))
		}
	}
	return "", didYouMean(suggest.Closest(ref, c.names, 2))
}
