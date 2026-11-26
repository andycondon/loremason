// Package registry loads the type registry and the record schemas.
package registry

import (
	"bytes"
	"fmt"
	"io/fs"
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

// Registry describes every record type.
type Registry struct {
	CommonEdges map[string]Edge  `yaml:"common_edges"`
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
}

// Type is one record type.
type Type struct {
	Kind            `yaml:",inline"`
	Prefix          string           `yaml:"prefix"`
	Layer           string           `yaml:"layer"`
	Revisable       bool             `yaml:"revisable"`
	RevisionsInline bool             `yaml:"revisions_inline"`
	Meaning         []string         `yaml:"meaning"`
	Subrecords      map[string]*Kind `yaml:"subrecords"` // by the field that lists them
	Template        string           `yaml:"template"`
	Body            string           `yaml:"body"`

	Schema *jsonschema.Schema `yaml:"-"`
}

// Edge is a kind of link.
type Edge struct {
	To         []string `yaml:"to"`
	Propagates bool     `yaml:"propagates"`
	Pinned     *bool    `yaml:"pinned"`
}

// Allows reports whether the edge may point at a node of kind k.
func (e Edge) Allows(k *Kind) bool {
	return slices.Contains(e.To, "any") || slices.Contains(e.To, k.Name)
}

// Folder is the folder under graph/ that holds the type's records.
func (t *Type) Folder() string { return strings.ReplaceAll(t.Name, "_", "-") }

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

// Load returns the registry built into the program.
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
