package record

import (
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Path says where a value is in a header: field names and list indexes. It
// prints the way a person would write it, such as support[0].direction.
type Path []string

// Field returns the path of a field inside the value at p.
func (p Path) Field(name string) Path { return append(slices.Clone(p), name) }

// Index returns the path of a list item inside the value at p.
func (p Path) Index(i int) Path { return append(slices.Clone(p), strconv.Itoa(i)) }

func (p Path) String() string {
	var b strings.Builder
	for _, part := range p {
		if _, err := strconv.Atoi(part); err == nil {
			b.WriteString("[" + part + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

// Map is a set of fields in a header: the header itself, or one entry in a
// list such as a belief's support. The zero Map has no fields, so a file
// that couldn't be read can still be asked about its fields.
type Map struct {
	node *yaml.Node
	Path Path
}

// Value is one field's value, and where it is.
type Value struct {
	node *yaml.Node
	Path Path
}

// Line is where the map starts.
func (m Map) Line() int {
	if m.node == nil {
		return 1
	}
	return m.node.Line
}

// Get returns a field's value.
func (m Map) Get(field string) (Value, bool) {
	if key, value := m.find(field); key != nil {
		return Value{value, m.Path.Field(field)}, true
	}
	return Value{}, false
}

// KeyLine is the line a field is written on, or the map's own line when the
// field isn't there.
func (m Map) KeyLine(field string) int {
	if key, _ := m.find(field); key != nil {
		return key.Line
	}
	return m.Line()
}

// Text returns a field's value when it's text, or "".
func (m Map) Text(field string) string {
	v, _ := m.Get(field)
	s, _ := v.Text()
	return s
}

// Entries returns the maps in a list field, skipping anything else.
func (m Map) Entries(field string) []Map {
	v, _ := m.Get(field)
	var out []Map
	for _, item := range v.Items() {
		if entry, ok := item.Map(); ok {
			out = append(out, entry)
		}
	}
	return out
}

func (m Map) find(field string) (key, value *yaml.Node) {
	if m.node == nil || m.node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.node.Content); i += 2 {
		if m.node.Content[i].Value == field {
			return m.node.Content[i], m.node.Content[i+1]
		}
	}
	return nil, nil
}

// Line is where the value is written.
func (v Value) Line() int { return v.node.Line }

// Text returns the value when it's text.
func (v Value) Text() (string, bool) {
	if v.node == nil || v.node.Kind != yaml.ScalarNode || v.node.Tag != "!!str" {
		return "", false
	}
	return v.node.Value, true
}

// Map returns the value when it's a set of fields.
func (v Value) Map() (Map, bool) {
	if v.node == nil || v.node.Kind != yaml.MappingNode {
		return Map{}, false
	}
	return Map{v.node, v.Path}, true
}

// Items returns a list's items. A value that isn't a list is its own only
// item, since a field such as relies_on may hold one link or a list of them.
func (v Value) Items() []Value {
	switch {
	case v.node == nil:
		return nil
	case v.node.Kind != yaml.SequenceNode:
		return []Value{v}
	}
	items := make([]Value, len(v.node.Content))
	for i, n := range v.node.Content {
		items[i] = Value{n, v.Path.Index(i)}
	}
	return items
}

// Decode stores the value in out, as yaml.Unmarshal would.
func (v Value) Decode(out any) error { return v.node.Decode(out) }
