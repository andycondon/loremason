// Package registry loads the type registry and the record schemas.
package registry

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/suggest"
	"github.com/andycondon/loremason/schema"
)

// The layers a type can belong to.
// FixedFPField is the field a fixed record keeps its fingerprint in.
const FixedFPField = "fp"

const (
	Knowing    = "knowing"
	Committing = "committing"
	Scope      = "scope"
)

// Registry describes every record type.
type Registry struct {
	CommonEdges map[string]Edge  `yaml:"common_edges"` // edges every record type has, such as mentions
	Types       map[string]*Type `yaml:"types"`

	byPrefix map[string]*Type
	byFolder map[string]*Type
	compiler *jsonschema.Compiler
}

// Kind is what a node in the graph can be: a record type, or a kind of
// sub-record such as support_link.
type Kind struct {
	Name     string          `yaml:"type"`     // a record type's name comes from its key in types.yaml
	Edges    map[string]Edge `yaml:"edges"`    // the links its fields may hold; a record type's include the common ones
	Identity []string        `yaml:"identity"` // fields that suggest two records are the same thing
	// Revisable kinds keep a revisions list; links can pin their revisions.
	// Sub-record kinds never are.
	Revisable bool `yaml:"revisable"`
	// Document marks a type whose records are prose we wrote, so evidence can
	// quote them. An edge's To may name `document` for all such types at once.
	Document bool `yaml:"document"`
}

// Type is one record type.
type Type struct {
	Kind            `yaml:",inline"`
	Prefix          string           `yaml:"prefix"`           // the ID prefix: bel for bel_7k3m9q
	Layer           string           `yaml:"layer"`            // Knowing, Committing or Scope
	RevisionsInline bool             `yaml:"revisions_inline"` // old wording is kept in the file, not left to git
	Meaning         []string         `yaml:"meaning"`          // fields a fingerprint covers; "body" for the body
	Fixed           bool             `yaml:"fixed"`            // the record never changes once written; it keeps its own fp (evidence)
	StatusFrom      string           `yaml:"status_from"`      // an edge of the type: a record with no status of its own takes its status from that edge's target (a step from its plan)
	Sealed          Sealed           `yaml:"sealed"`           // when the current revision is sealed by state, not only by a pin
	Closed          []string         `yaml:"closed"`           // statuses meaning the record is no longer in force; each is one of the schema's statuses
	Fallen          []string         `yaml:"fallen"`           // statuses meaning the record no longer holds; each is one of the schema's statuses
	Subrecords      map[string]*Kind `yaml:"subrecords"`       // by the field that lists them
	Template        string           `yaml:"template"`         // header lines `mason new` writes after id and title
	Body            string           `yaml:"body"`             // the body `mason new` writes

	Schema *jsonschema.Schema `yaml:"-"`
	// Settleable is set when a link can settle the type's records: some edge
	// that settles names it as a target (a question).
	Settleable bool `yaml:"-"`
}

// Sealed says when a record's current revision is sealed by state. Status
// lists the statuses that seal it, those of the record that holds its status
// (its own, or the one its type's StatusFrom leads to). Settled seals a record
// once a link settles it (a question). The zero value seals nothing.
type Sealed struct {
	Status  []string `yaml:"status"`
	Settled bool     `yaml:"settled"`
}

// Edge is a kind of link.
type Edge struct {
	To         []string `yaml:"to"`         // the types it may point at, or any
	Propagates bool     `yaml:"propagates"` // a change in the target means the record needs a re-check
	Pinned     *bool    `yaml:"pinned"`     // whether the link pins its target; nil means as Propagates
	Settles    []string `yaml:"settles"`    // the source's statuses at which the link settles its target; each is one of the source type's statuses
	Pending    []string `yaml:"pending"`    // the source's statuses at which the link is an answer still waiting; each is one of the source type's statuses
	// Constraints the check reports as warnings.
	Within       string   `yaml:"within"`        // another edge of the same type: the target's parent record must be a target of the source's link by that edge
	TargetStatus []string `yaml:"target_status"` // the statuses the target should be in; each is one of every target type's statuses
	OneInForce   bool     `yaml:"one_in_force"`  // at most one source in force (and, for an edge that settles, settling) per target
}

// ShouldPin reports whether a link of this edge to a node of kind target
// records the target's revision and fingerprint: the edge is pinned, and the
// target keeps revisions.
func (e Edge) ShouldPin(target *Kind) bool {
	pinned := e.Propagates
	if e.Pinned != nil {
		pinned = *e.Pinned
	}
	return pinned && target.Revisable
}

// Allows reports whether the edge may point at a node of kind k.
func (e Edge) Allows(k *Kind) bool {
	return slices.Contains(e.To, "any") || slices.Contains(e.To, k.Name) || (k.Document && slices.Contains(e.To, "document"))
}

// Folder is the folder under graph/ that holds the type's records.
func (t *Type) Folder() string { return strings.ReplaceAll(t.Name, "_", "-") }

// MeaningFields are the header fields a fingerprint covers. Whether the body
// counts too is BodyIsMeaning.
func (t *Type) MeaningFields() []string {
	var fields []string
	for _, f := range t.Meaning {
		if f != "body" {
			fields = append(fields, f)
		}
	}
	return fields
}

// BodyIsMeaning reports whether a fingerprint covers the body, as for
// documents, rather than only header fields, as for beliefs.
func (t *Type) BodyIsMeaning() bool { return slices.Contains(t.Meaning, "body") }

// IsFallen reports whether status is one the type lists as fallen, so a
// commitment relying on a record in it needs another look.
func (t *Type) IsFallen(status string) bool { return slices.Contains(t.Fallen, status) }

// IsClosed reports whether status is one the type lists as closed, so a
// record in it is no longer in force and a fallen target doesn't reopen it.
func (t *Type) IsClosed(status string) bool { return slices.Contains(t.Closed, status) }

// field returns the type's schema for a header field, following references,
// or nil when the type has no such field.
func (t *Type) field(name string) *jsonschema.Schema {
	s := t.Schema.Properties[name]
	for s != nil && s.Enum == nil && s.Default == nil && s.Ref != nil {
		s = s.Ref
	}
	return s
}

// SingleLink reports whether name is an edge of the type whose field takes one
// link, not a list of them, such as generated_by.
func (t *Type) SingleLink(name string) bool {
	s := t.field(name)
	_, isEdge := t.Edges[name]
	return isEdge && s != nil && s.Types != nil && !slices.Contains(s.Types.ToStrings(), "array")
}

// Default returns the default the schema declares for a header field, such
// as todo for a task's progress, or "" when it declares none.
func (t *Type) Default(field string) string {
	s := t.field(field)
	if s == nil || s.Default == nil {
		return ""
	}
	str, _ := (*s.Default).(string)
	return str
}

// statuses lists the values the type's schema allows for status, or nil when
// it has no status or doesn't list them.
func (t *Type) statuses() []string {
	s := t.field("status")
	if s == nil || s.Enum == nil {
		return nil
	}
	var out []string
	for _, v := range s.Enum.Values {
		if str, ok := v.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

// checkStatuses reports a fallen or closed status the type's schema doesn't
// allow, and a settles or pending status on one of the type's edges that it
// doesn't allow either.
func (t *Type) checkStatuses() error {
	for _, k := range []struct {
		key  string
		list []string
	}{{"fallen", t.Fallen}, {"closed", t.Closed}} {
		for _, st := range k.list {
			if !slices.Contains(t.statuses(), st) {
				return fmt.Errorf("types.yaml: %s lists `%s` as %s, but its schema has no such status", t.Name, st, k.key)
			}
		}
	}
	for _, st := range t.Fallen {
		if slices.Contains(t.Closed, st) {
			return fmt.Errorf("types.yaml: %s lists `%s` as both fallen and closed; a status is one or the other", t.Name, st)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(t.Edges)) {
		e := t.Edges[name]
		for _, k := range []struct {
			key  string
			list []string
		}{{"settles", e.Settles}, {"pending", e.Pending}} {
			for _, st := range k.list {
				if !slices.Contains(t.statuses(), st) {
					return fmt.Errorf("types.yaml: the %s edge of %s lists `%s` as %s, but its schema has no such status", name, t.Name, st, k.key)
				}
			}
		}
	}
	return nil
}

// checkSealed reports a status_from that isn't an edge of the type, and a
// sealed status that neither the type's schema nor, with status_from, a target
// of that edge allows.
func (r *Registry) checkSealed(t *Type) error {
	s := t.Sealed
	statuses := t.statuses()
	if t.StatusFrom != "" {
		e, ok := t.Edges[t.StatusFrom]
		if !ok {
			return fmt.Errorf("types.yaml: %s takes its status from `%s`, but it has no such edge", t.Name, t.StatusFrom)
		}
		for _, to := range e.To {
			if target := r.Types[to]; target != nil {
				statuses = append(statuses, target.statuses()...)
			}
		}
	}
	for _, st := range s.Status {
		if !slices.Contains(statuses, st) {
			return fmt.Errorf("types.yaml: %s is sealed at `%s`, but its schema has no such status", t.Name, st)
		}
	}
	return nil
}

// checkConstraints reports a within that isn't another edge of the type, and
// a target_status that a target type's schema doesn't allow.
func (r *Registry) checkConstraints(t *Type) error {
	for _, name := range slices.Sorted(maps.Keys(t.Edges)) {
		e := t.Edges[name]
		if e.Within != "" {
			if _, ok := t.Edges[e.Within]; !ok || e.Within == name {
				return fmt.Errorf("types.yaml: the %s edge of %s is within `%s`, but %s has no such other edge", name, t.Name, e.Within, t.Name)
			}
			for _, to := range e.To {
				if !r.isSubrecordKind(to) {
					return fmt.Errorf("types.yaml: the %s edge of %s is within `%s`, so each target must be a sub-record type, not `%s`", name, t.Name, e.Within, to)
				}
			}
		}
		for _, st := range e.TargetStatus {
			for _, to := range e.To {
				target := r.Types[to]
				if target == nil {
					return fmt.Errorf("types.yaml: the %s edge of %s has a target_status, so each target must be a record type, not `%s`", name, t.Name, to)
				}
				if !slices.Contains(target.statuses(), st) {
					return fmt.Errorf("types.yaml: the %s edge of %s lists `%s` as target_status, but %s has no such status", name, t.Name, st, to)
				}
			}
		}
	}
	return nil
}

// isSubrecordKind reports whether a name is a kind of sub-record, such as
// option, of some record type.
func (r *Registry) isSubrecordKind(name string) bool {
	for _, t := range r.Types {
		for _, k := range t.Subrecords {
			if k.Name == name {
				return true
			}
		}
	}
	return false
}

// checkDocument reports the first rule a document type breaks. Documents all
// have the same shape, so any of them can be quoted or attributed the same way.
func (t *Type) checkDocument() error {
	if !t.Document {
		return nil
	}
	bad := func(rule string) error {
		return fmt.Errorf("types.yaml: %s is a document, so %s", t.Name, rule)
	}
	if !t.Revisable || t.RevisionsInline {
		return bad("it must be revisable, with its revisions left to git (revisions_inline: false)")
	}
	if !slices.Contains(t.Meaning, "title") || !t.BodyIsMeaning() {
		return bad("its meaning must cover title and body")
	}
	for _, l := range []struct{ edge, to string }{{"generated_by", "activity"}, {"attributed_to", "actor"}} {
		if !slices.Contains(t.Edges[l.edge].To, l.to) {
			return bad(fmt.Sprintf("it must have a %s edge to %s", l.edge, l.to))
		}
		if t.Schema.Properties[l.edge] == nil {
			return bad(fmt.Sprintf("its schema must have a %s field", l.edge))
		}
	}
	return nil
}

// checkFixed reports a fixed type that is also revisable, since a record is
// one or the other, and one whose schema has no fp field to keep the
// fingerprint in.
func (t *Type) checkFixed() error {
	switch {
	case !t.Fixed:
		return nil
	case t.Revisable:
		return fmt.Errorf("types.yaml: %s is fixed, so it can't also be revisable; a record is either revisable or fixed", t.Name)
	case t.Schema.Properties[FixedFPField] == nil:
		return fmt.Errorf("types.yaml: %s is fixed, so its schema must have an fp field", t.Name)
	}
	return nil
}

// Validate checks a record's fields, as plain values, against the type's schema.
func (t *Type) Validate(fields map[string]any) error { return t.Schema.Validate(fields) }

// Find returns a type by its name or its folder's name, so design_doc and
// design-doc both work. When there's none, the error suggests what was meant.
func (r *Registry) Find(name string) (*Type, error) {
	if t := r.Types[name]; t != nil {
		return t, nil
	}
	if t := r.byFolder[name]; t != nil {
		return t, nil
	}
	if s := suggest.Near(name, r.Names()); s != "" {
		return nil, fmt.Errorf("unknown record type `%s`; did you mean `%s`?", name, s)
	}
	return nil, fmt.Errorf("unknown record type `%s`; the types are %s", name, strings.Join(r.Names(), ", "))
}

// TypeOfID returns the type an ID's prefix names, or nil when the text isn't
// an ID or no type has its prefix.
func (r *Registry) TypeOfID(s string) *Type {
	prefix, ok := id.Prefix(s)
	if !ok {
		return nil
	}
	return r.byPrefix[prefix]
}

// ByPrefix returns the type with an ID prefix, or nil.
func (r *Registry) ByPrefix(p string) *Type { return r.byPrefix[p] }

// ByFolder returns the type a folder holds, or nil.
func (r *Registry) ByFolder(f string) *Type { return r.byFolder[f] }

// Folders lists the type folders in order.
func (r *Registry) Folders() []string {
	var folders []string
	for _, t := range r.Types {
		folders = append(folders, t.Folder())
	}
	slices.Sort(folders)
	return folders
}

// Names lists the type names in order.
func (r *Registry) Names() []string {
	var names []string
	for n := range r.Types {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// SubschemaAt returns the compiled schema at a location a validation error
// names, or nil.
func (r *Registry) SubschemaAt(url string) *jsonschema.Schema {
	s, err := r.compiler.Compile(url)
	if err != nil {
		return nil
	}
	return s
}

// Load returns the registry built into the program. It panics if the
// embedded schemas don't compile, which the tests rule out.
func Load() *Registry { return load() }

var load = sync.OnceValue(func() *Registry {
	r, err := parse()
	if err != nil {
		panic("the embedded schemas are broken: " + err.Error())
	}
	return r
})

func parse() (*Registry, error) {
	data, err := schema.FS.ReadFile("types.yaml")
	if err != nil {
		return nil, err
	}
	r := &Registry{byPrefix: map[string]*Type{}, byFolder: map[string]*Type{}, compiler: jsonschema.NewCompiler()}
	r.compiler.RegisterFormat(&jsonschema.Format{Name: "calendar-date", Validate: calendarDate})
	r.compiler.AssertFormat()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(r); err != nil {
		return nil, fmt.Errorf("types.yaml: %w", err)
	}
	files, err := fs.Glob(schema.FS, "*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		data, err := schema.FS.ReadFile(f)
		if err != nil {
			return nil, err
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := r.compiler.AddResource(schema.BaseURL+f, doc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	for name, t := range r.Types {
		t.Name = name
		if t.Edges == nil {
			t.Edges = map[string]Edge{}
		}
		for e, edge := range r.CommonEdges {
			t.Edges[e] = edge
		}
		if other := r.byPrefix[t.Prefix]; other != nil {
			return nil, fmt.Errorf("%s and %s share the prefix %s", name, other.Name, t.Prefix)
		}
		r.byPrefix[t.Prefix], r.byFolder[t.Folder()] = t, t
		if t.Schema, err = r.compiler.Compile(schema.BaseURL + t.Folder() + ".json"); err != nil {
			return nil, err
		}
		if err := t.checkStatuses(); err != nil {
			return nil, err
		}
		if err := t.checkDocument(); err != nil {
			return nil, err
		}
		if err := t.checkFixed(); err != nil {
			return nil, err
		}
	}
	for _, t := range r.Types {
		for _, e := range t.Edges {
			if len(e.Settles) == 0 {
				continue
			}
			for _, to := range e.To {
				if target := r.Types[to]; target != nil {
					target.Settleable = true
				}
			}
		}
	}
	for _, name := range r.Names() {
		if err := r.checkSealed(r.Types[name]); err != nil {
			return nil, err
		}
		if err := r.checkConstraints(r.Types[name]); err != nil {
			return nil, err
		}
	}
	return r, nil
}

var looksLikeDate = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}`)

// calendarDate checks that text starting with a date names a real day, such
// as no 30 February. The schema's pattern checks the rest of the shape.
func calendarDate(v any) error {
	s, ok := v.(string)
	if !ok || !looksLikeDate.MatchString(s) {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, s[:len(time.DateOnly)]); err != nil {
		return fmt.Errorf("there's no such day")
	}
	return nil
}
