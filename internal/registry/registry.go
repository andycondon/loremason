// Package registry loads the type registry and the record schemas.
package registry

import (
	"bytes"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

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

// Type is one record type.
type Type struct {
	Name            string               `yaml:"-"`
	Prefix          string               `yaml:"prefix"`
	Layer           string               `yaml:"layer"`
	Revisable       bool                 `yaml:"revisable"`
	RevisionsInline bool                 `yaml:"revisions_inline"`
	Meaning         []string             `yaml:"meaning"`
	Identity        []string             `yaml:"identity"`
	Edges           map[string]Edge      `yaml:"edges"` // including the common ones
	Subrecords      map[string]Subrecord `yaml:"subrecords"`
	Template        string               `yaml:"template"`
	Body            string               `yaml:"body"`

	Schema *jsonschema.Schema `yaml:"-"`
}

// Edge is a kind of link.
type Edge struct {
	To         []string `yaml:"to"`
	Propagates bool     `yaml:"propagates"`
	Pinned     *bool    `yaml:"pinned"`
}

// Allows reports whether the edge may point at the named type.
func (e Edge) Allows(target string) bool {
	return slices.Contains(e.To, "any") || slices.Contains(e.To, target)
}

// Subrecord is a kind of record kept in a list inside another.
type Subrecord struct {
	Type  string          `yaml:"type"`
	Edges map[string]Edge `yaml:"edges"`
}

// Folder is the folder under graph/ that holds the type's records.
func (t *Type) Folder() string { return strings.ReplaceAll(t.Name, "_", "-") }

// ByPrefix returns the type with an ID prefix, or nil.
func (r *Registry) ByPrefix(p string) *Type { return r.byPrefix[p] }

// ByFolder returns the type a folder holds, or nil.
func (r *Registry) ByFolder(f string) *Type { return r.byFolder[f] }

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
