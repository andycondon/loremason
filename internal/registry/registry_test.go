package registry

import (
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/andycondon/loremason/schema"
)

// The tests follow registry.go: edges, types, the registry's lookups,
// loading types.yaml and the schemas, then dates.

// typ returns a type by name, or fails.
func typ(t *testing.T, name string) *Type {
	t.Helper()
	typ, err := Load().Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

// ---- Edge ----

func TestEdgeShouldPin(t *testing.T) {
	task, evidence := typ(t, "task"), typ(t, "evidence")
	for _, c := range []struct {
		edge   Edge
		target string
		want   bool
	}{
		{task.Edges["relies_on"], "belief", true},    // propagates
		{task.Edges["part_of"], "initiative", false}, // doesn't propagate
		{task.Edges["mentions"], "belief", false},    // never
		{evidence.Edges["from"], "finding", true},    // pinned, though it doesn't propagate
		{evidence.Edges["from"], "artifact", false},  // artifacts have no revisions
	} {
		if got := c.edge.ShouldPin(&typ(t, c.target).Kind); got != c.want {
			t.Errorf("%+v to %s: got %v", c.edge, c.target, got)
		}
	}
}

func TestEdgeAllows(t *testing.T) {
	answers := typ(t, "belief").Edges["answers"]
	if !answers.Allows(&typ(t, "question").Kind) || answers.Allows(&typ(t, "task").Kind) {
		t.Error("answers points at questions only")
	}
	mentions := typ(t, "belief").Edges["mentions"]
	if !mentions.Allows(&typ(t, "task").Kind) || !mentions.Allows(typ(t, "belief").Subrecords["support"]) {
		t.Error("mentions points at any kind")
	}
}

// ---- Type ----

func TestTypeFolder(t *testing.T) {
	if typ(t, "design_doc").Folder() != "design-doc" || typ(t, "belief").Folder() != "belief" {
		t.Error("a folder is the type's name in kebab case")
	}
}

func TestTypeMeaning(t *testing.T) {
	for _, c := range []struct {
		name   string
		fields []string
		body   bool
	}{
		{"belief", []string{"claim"}, false},
		{"design_doc", []string{"title"}, true},
		{"evidence", nil, true},
	} {
		typ := typ(t, c.name)
		if !slices.Equal(typ.MeaningFields(), c.fields) || typ.BodyIsMeaning() != c.body {
			t.Errorf("%s: fields %v, body %v", c.name, typ.MeaningFields(), typ.BodyIsMeaning())
		}
	}
}

func TestTypeValidate(t *testing.T) {
	belief := typ(t, "belief")
	fields := func(created string) map[string]any {
		return map[string]any{"id": "bel_7k3m9q", "title": "T", "claim": "C", "status": "held", "asserted_by": "agent", "created": created}
	}
	if err := belief.Validate(fields("2024-02-29")); err != nil {
		t.Errorf("a valid belief: %v", err)
	}
	if err := belief.Validate(fields("2026-02-29")); err == nil || !strings.Contains(err.Error(), "no such day") {
		t.Errorf("29 February 2026: %v", err)
	}
	missing := fields("2026-11-22")
	delete(missing, "claim")
	if err := belief.Validate(missing); err == nil {
		t.Error("a belief needs a claim")
	}
}

// ---- Registry: finding a type ----

func TestRegistryFind(t *testing.T) {
	r := Load()
	for _, name := range []string{"design_doc", "design-doc"} {
		if typ, err := r.Find(name); err != nil || typ.Name != "design_doc" {
			t.Errorf("Find(%q) = %v, %v", name, typ, err)
		}
	}
	for name, want := range map[string]string{
		"beleif":     "did you mean `belief`?",
		"xyzzyplugh": "the types are activity, actor,",
	} {
		if _, err := r.Find(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Find(%q): %v", name, err)
		}
	}
}

func TestRegistryTypeOfID(t *testing.T) {
	r := Load()
	for s, want := range map[string]string{
		"bel_7k3m9q":  "belief",
		"actv_7k3m9q": "activity",
		"zzz_7k3m9q":  "", // no type has the prefix
		"bel":         "", // not an ID
		"bel_7K3M9Q":  "",
	} {
		got := ""
		if typ := r.TypeOfID(s); typ != nil {
			got = typ.Name
		}
		if got != want {
			t.Errorf("TypeOfID(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestRegistryByPrefixAndByFolder(t *testing.T) {
	r := Load()
	if r.ByPrefix("dd") != typ(t, "design_doc") || r.ByPrefix("zzz") != nil {
		t.Error("ByPrefix")
	}
	if r.ByFolder("design-doc") != typ(t, "design_doc") || r.ByFolder("design_doc") != nil {
		t.Error("ByFolder takes the folder's name, not the type's")
	}
}

func TestRegistryFoldersAndNames(t *testing.T) {
	r := Load()
	folders, names := r.Folders(), r.Names()
	if len(folders) != len(r.Types) || len(names) != len(r.Types) || !slices.IsSorted(folders) || !slices.IsSorted(names) {
		t.Fatalf("folders %v, names %v", folders, names)
	}
	if !slices.Contains(folders, "design-doc") || !slices.Contains(names, "design_doc") {
		t.Error("folders are in kebab case, names as types.yaml has them")
	}
}

func TestRegistrySubschemaAt(t *testing.T) {
	r := Load()
	if r.SubschemaAt(schema.BaseURL+"common.json#/$defs/ref") == nil {
		t.Error("a schema that exists")
	}
	if r.SubschemaAt(schema.BaseURL+"common.json#/$defs/nothing") != nil {
		t.Error("a schema that doesn't")
	}
}

// ---- Loading types.yaml and the schemas ----

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

// Every type has the edges every record type shares.
func TestCommonEdgesAreOnEveryType(t *testing.T) {
	r := Load()
	for name, typ := range r.Types {
		for e, edge := range r.CommonEdges {
			if got, ok := typ.Edges[e]; !ok || !slices.Equal(got.To, edge.To) {
				t.Errorf("%s has no %s edge", name, e)
			}
		}
	}
}

// Pins are written only in a record's own link fields, so no sub-record's
// edge may need one.
func TestSubrecordEdgesAreNeverPinned(t *testing.T) {
	r := Load()
	for name, typ := range r.Types {
		for field, sub := range typ.Subrecords {
			for e, edge := range sub.Edges {
				for _, to := range r.Types {
					if edge.Allows(&to.Kind) && edge.ShouldPin(&to.Kind) {
						t.Errorf("%s.%s.%s would pin %s", name, field, e, to.Name)
					}
				}
			}
		}
	}
}

// ---- Dates ----

func TestCalendarDate(t *testing.T) {
	for v, ok := range map[any]bool{
		"2026-11-22":           true,
		"2024-02-29":           true,
		"2026-02-29":           false,
		"2026-02-30T10:00:00Z": false,
		"22 November":          true, // not a date: the pattern's to reject
		42:                     true,
	} {
		if err := calendarDate(v); (err == nil) != ok {
			t.Errorf("calendarDate(%v): %v", v, err)
		}
	}
}
