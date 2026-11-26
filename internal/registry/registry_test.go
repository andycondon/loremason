package registry

import (
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// isLinks reports whether a property uses a shared link definition.
func isLinks(s *jsonschema.Schema) bool {
	return s.Ref != nil && (strings.HasSuffix(s.Ref.Location, "/links") || strings.HasSuffix(s.Ref.Location, "/ref"))
}

func TestRegistryAndSchemasAgree(t *testing.T) {
	r := Load()
	known := []string{"any"}
	for name, typ := range r.Types {
		known = append(known, name)
		for _, sub := range typ.Subrecords {
			known = append(known, sub.Name)
		}
	}
	checkTargets := func(where string, edges map[string]Edge) {
		for e, edge := range edges {
			for _, to := range edge.To {
				if !slices.Contains(known, to) {
					t.Errorf("%s.%s points at unknown type %q", where, e, to)
				}
			}
		}
	}
	for name, typ := range r.Types {
		props := typ.Schema.Properties
		checkTargets(name, typ.Edges)
		for e := range typ.Edges {
			if props[e] == nil {
				t.Errorf("%s: edge %s isn't in the schema", name, e)
			}
		}
		for p, s := range props {
			if _, ok := typ.Edges[p]; isLinks(s) && !ok {
				t.Errorf("%s: link field %s isn't in the registry", name, p)
			}
		}
		for _, f := range slices.Concat(typ.Meaning, typ.Identity) {
			if f != "body" && props[f] == nil {
				t.Errorf("%s: meaning or identity field %s isn't in the schema", name, f)
			}
		}
		if (props["revisions"] != nil) != typ.Revisable {
			t.Errorf("%s: revisable and the schema's revisions field disagree", name)
		}
		if typ.Revisable {
			entry := props["revisions"].Items2020.Properties
			if (entry["commit"] == nil) != typ.RevisionsInline {
				t.Errorf("%s: revisions_inline and the schema's commit field disagree", name)
			}
			for _, f := range typ.Meaning {
				if typ.RevisionsInline && entry[f] == nil {
					t.Errorf("%s: revisions can't keep old %s", name, f)
				}
			}
		}
		for field, sub := range typ.Subrecords {
			checkTargets(name+"."+field, sub.Edges)
			entry := props[field].Items2020.Properties
			for e := range sub.Edges {
				if entry[e] == nil {
					t.Errorf("%s.%s: edge %s isn't in the schema", name, field, e)
				}
			}
			for p, s := range entry {
				if _, ok := sub.Edges[p]; isLinks(s) && !ok {
					t.Errorf("%s.%s: link field %s isn't in the registry", name, field, p)
				}
			}
		}
	}
}
