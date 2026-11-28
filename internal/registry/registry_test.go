package registry

import (
	"maps"
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

func TestTypeFallen(t *testing.T) {
	belief := typ(t, "belief")
	for status, want := range map[string]bool{"incorrect": true, "insufficient": true, "held": false, "withdrawn": false, "": false} {
		if belief.IsFallen(status) != want {
			t.Errorf("IsFallen(%q) = %v", status, !want)
		}
	}
	if typ(t, "task").IsFallen("rejected") {
		t.Error("a type with no fallen statuses has none fall")
	}
}

func TestTypeCheckFallen(t *testing.T) {
	belief, task := *typ(t, "belief"), *typ(t, "task")
	if err := belief.checkStatuses(); err != nil {
		t.Errorf("belief's own fallen statuses: %v", err)
	}
	belief.Fallen = []string{"incorrect", "gone"}
	if err := belief.checkStatuses(); err == nil || !strings.Contains(err.Error(), "`gone`") {
		t.Errorf("a status the schema lacks: %v", err)
	}
	task.Fallen = []string{"rejected"} // task's status comes from a shared definition
	task.Closed = nil
	if err := task.checkStatuses(); err != nil {
		t.Errorf("a status through a $ref: %v", err)
	}
	task.Closed = []string{"rejected"}
	if err := task.checkStatuses(); err == nil || !strings.Contains(err.Error(), "both fallen and closed") {
		t.Errorf("a status both fallen and closed: %v", err)
	}
	task.Closed = nil
	task.Fallen = []string{"held"}
	if err := task.checkStatuses(); err == nil {
		t.Error("held isn't a task status")
	}
	finding := *typ(t, "finding") // no status at all
	finding.Fallen = []string{"incorrect"}
	if err := finding.checkStatuses(); err == nil {
		t.Error("a type with no status can't have a fallen one")
	}
}

func TestTypeClosed(t *testing.T) {
	task := typ(t, "task")
	for status, want := range map[string]bool{"rejected": true, "deprecated": true, "superseded": true, "accepted": false, "": false} {
		if task.IsClosed(status) != want {
			t.Errorf("IsClosed(%q) = %v", status, !want)
		}
	}
	if !typ(t, "belief").IsClosed("withdrawn") || typ(t, "belief").IsClosed("incorrect") {
		t.Error("a belief is closed once withdrawn, not when incorrect")
	}
	if typ(t, "step").IsClosed("rejected") {
		t.Error("a type with no closed statuses has none closed")
	}
}

// A within or target_status that the type or its targets don't allow is refused.
func TestRegistryRefusesBadConstraints(t *testing.T) {
	r := Load()
	task := *r.Types["task"]
	task.Edges = maps.Clone(task.Edges)
	e := task.Edges["implements"]
	e.Within = "nope"
	task.Edges["implements"] = e
	if err := r.checkConstraints(&task); err == nil || !strings.Contains(err.Error(), "within `nope`") {
		t.Errorf("within naming an unknown edge: %v", err)
	}
	e.Within = ""
	e.TargetStatus = []string{"gone"}
	task.Edges["implements"] = e
	if err := r.checkConstraints(&task); err == nil || !strings.Contains(err.Error(), "`gone`") {
		t.Errorf("target_status with an unknown status: %v", err)
	}
	e.TargetStatus = []string{"accepted"}
	task.Edges["implements"] = e
	if err := r.checkConstraints(&task); err != nil {
		t.Errorf("a known status: %v", err)
	}
	dec := *r.Types["decision"]
	dec.Edges = maps.Clone(dec.Edges)
	c := dec.Edges["chooses"]
	c.To = []string{"question"}
	dec.Edges["chooses"] = c
	if err := r.checkConstraints(&dec); err == nil || !strings.Contains(err.Error(), "must be a sub-record type, not `question`") {
		t.Errorf("within an edge whose target isn't a sub-record type: %v", err)
	}
}

func TestTypeCheckClosed(t *testing.T) {
	task := *typ(t, "task")
	if err := task.checkStatuses(); err != nil {
		t.Errorf("task's own closed statuses: %v", err)
	}
	task.Closed = []string{"rejected", "gone"}
	want := "types.yaml: task lists `gone` as closed, but its schema has no such status"
	if err := task.checkStatuses(); err == nil || err.Error() != want {
		t.Errorf("a closed status the schema lacks: %v", err)
	}
}

func TestTypeCheckEdgeStatuses(t *testing.T) {
	// withEdge returns a copy of a type whose answers edge is changed by set.
	withEdge := func(name string, set func(*Edge)) *Type {
		c := *typ(t, name)
		c.Edges = map[string]Edge{}
		for k, e := range typ(t, name).Edges {
			c.Edges[k] = e
		}
		e := c.Edges["answers"]
		set(&e)
		c.Edges["answers"] = e
		return &c
	}
	for _, name := range []string{"decision", "belief"} {
		if err := typ(t, name).checkStatuses(); err != nil {
			t.Errorf("%s's own settles and pending statuses: %v", name, err)
		}
	}
	for _, c := range []struct{ key, want string }{
		{"settles", "types.yaml: the answers edge of decision lists `gone` as settles, but its schema has no such status"},
		{"pending", "types.yaml: the answers edge of decision lists `gone` as pending, but its schema has no such status"},
	} {
		d := withEdge("decision", func(e *Edge) {
			if c.key == "settles" {
				e.Settles = []string{"accepted", "gone"}
			} else {
				e.Pending = []string{"gone"}
			}
		})
		if err := d.checkStatuses(); err == nil || err.Error() != c.want {
			t.Errorf("an unknown status in %s: %v", c.key, err)
		}
	}
	// The statuses are the source's: held is a belief's, not a decision's.
	if err := withEdge("decision", func(e *Edge) { e.Settles = []string{"held"} }).checkStatuses(); err == nil {
		t.Error("held isn't a decision status")
	}
	if err := withEdge("belief", func(e *Edge) { e.Settles = []string{"accepted"} }).checkStatuses(); err == nil {
		t.Error("accepted isn't a belief status")
	}
}

func TestTypeEdgeSettlesAndPending(t *testing.T) {
	dec, bel := typ(t, "decision").Edges["answers"], typ(t, "belief").Edges["answers"]
	if !slices.Equal(dec.Settles, []string{"accepted"}) || !slices.Equal(dec.Pending, []string{"proposed", "leaning"}) {
		t.Errorf("a decision's answers: settles %v, pending %v", dec.Settles, dec.Pending)
	}
	if !slices.Equal(bel.Settles, []string{"held"}) || len(bel.Pending) != 0 {
		t.Errorf("a belief's answers: settles %v, pending %v", bel.Settles, bel.Pending)
	}
}

func TestTypeDefault(t *testing.T) {
	task := typ(t, "task")
	for _, c := range []struct {
		typ   *Type
		field string
		want  string
	}{
		{task, "progress", "todo"},
		{task, "status", ""},  // a field with no default
		{task, "nothing", ""}, // no such field
		{typ(t, "belief"), "progress", ""},
	} {
		if got := c.typ.Default(c.field); got != c.want {
			t.Errorf("%s Default(%q) = %q, want %q", c.typ.Name, c.field, got, c.want)
		}
	}
}

func TestEveryTypeHasAKnownLayer(t *testing.T) {
	for name, typ := range Load().Types {
		if !slices.Contains([]string{Knowing, Committing, Scope}, typ.Layer) {
			t.Errorf("%s: layer %q", name, typ.Layer)
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
	known := []string{"any", "document"}
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
		"2026-11-22T24:00Z":    false,
		"2026-11-22T25:00Z":    false,
		"2026-11-22T10:00:60Z": false,
		"2026-11-22 10:00 Z":   true,
		"2026-11-22t10:00:00z": true,
		"22 November":          true, // not a date: the pattern's to reject
		42:                     true,
	} {
		if err := calendarDate(v); (err == nil) != ok {
			t.Errorf("calendarDate(%v): %v", v, err)
		}
	}
}

// ---- Documents ----

var documentTypes = []string{"finding", "design_doc", "prd", "plan"}

func TestEdgeAllowsDocument(t *testing.T) {
	from := typ(t, "evidence").Edges["from"]
	for _, name := range documentTypes {
		if !from.Allows(&typ(t, name).Kind) {
			t.Errorf("evidence can't come from %s", name)
		}
	}
	for _, name := range []string{"actor", "artifact"} {
		if !from.Allows(&typ(t, name).Kind) {
			t.Errorf("evidence can't come from %s", name)
		}
	}
	for _, name := range []string{"decision", "task", "belief"} {
		if from.Allows(&typ(t, name).Kind) {
			t.Errorf("evidence can come from %s, which isn't a document", name)
		}
	}
}

func TestCheckDocumentPassesForTheDocumentTypes(t *testing.T) {
	for _, name := range documentTypes {
		typ := typ(t, name)
		if !typ.Document {
			t.Errorf("%s isn't marked a document", name)
		}
		if err := typ.checkDocument(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if typ(t, "task").Document || typ(t, "decision").Document {
		t.Error("only finding, design doc, PRD and plan are documents")
	}
}

func TestCheckDocumentFindsABrokenShape(t *testing.T) {
	// A copy of design_doc, broken one way at a time.
	broken := func(change func(*Type)) error {
		d := *typ(t, "design_doc")
		d.Edges = maps.Clone(d.Edges)
		d.Meaning = slices.Clone(d.Meaning)
		change(&d)
		return d.checkDocument()
	}
	for _, c := range []struct {
		name   string
		change func(*Type)
		want   string
	}{
		{"a missing edge", func(d *Type) { delete(d.Edges, "generated_by") }, "generated_by edge to activity"},
		{"an edge to the wrong type", func(d *Type) { d.Edges["attributed_to"] = Edge{To: []string{"artifact"}} }, "attributed_to edge to actor"},
		{"a missing schema field", func(d *Type) { d.Schema = typ(t, "task").Schema }, "schema must have a generated_by field"},
		{"no body in meaning", func(d *Type) { d.Meaning = []string{"title"} }, "title and body"},
		{"not revisable", func(d *Type) { d.Revisable = false }, "revisable"},
		{"revisions kept inline", func(d *Type) { d.RevisionsInline = true }, "left to git"},
	} {
		err := broken(c.change)
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "design_doc") {
			t.Errorf("%s: got %v, want one naming design_doc and %q", c.name, err, c.want)
		}
	}
}
