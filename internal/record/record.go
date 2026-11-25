// Package record reads record files: a YAML header between --- lines, then a
// markdown body.
package record

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Record is one parsed file.
type Record struct {
	Header Map            // the header's fields; line numbers are the file's own
	Data   map[string]any // the same fields as plain values, for the JSON Schema check
	Body   string
}

// Error is a problem at a line of the file.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

var yamlLine = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)`)

// Parse reads a record file.
func Parse(data []byte) (*Record, []*Error) {
	fail := func(line int, format string, args ...any) (*Record, []*Error) {
		return nil, []*Error{{line, fmt.Sprintf(format, args...)}}
	}
	if !utf8.Valid(data) {
		line := 1
		for rest := data; ; {
			r, n := utf8.DecodeRune(rest)
			if r == utf8.RuneError && n <= 1 {
				break
			}
			if r == '\n' {
				line++
			}
			rest = rest[n:]
		}
		return fail(line, "the file isn't valid UTF-8")
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff")) // a byte order mark, as some editors write
	header, body, ok := split(data)
	if !ok {
		return fail(1, "the file must start with a YAML header between two `---` lines")
	}
	// A blank line stands in for the opening ---, so YAML lines match the file.
	var doc yaml.Node
	if err := yaml.Unmarshal(append([]byte("\n"), header...), &doc); err != nil {
		m := yamlLine.FindStringSubmatch(err.Error())
		if m == nil {
			return fail(1, "the header isn't valid YAML: %v", err)
		}
		line, _ := strconv.Atoi(m[1])
		return fail(max(line, 1), "the header isn't valid YAML: %s", m[2])
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fail(1, "the header must be a list of `field: value` lines")
	}
	r := &Record{Header: Map{node: doc.Content[0]}, Body: string(body)}
	var c converter
	r.Data, _ = c.value(doc.Content[0]).(map[string]any)
	return r, c.errs
}

// split separates the header from the body.
func split(data []byte) (header, body []byte, ok bool) {
	first, rest, found := bytes.Cut(data, []byte("\n"))
	if !found || !isFence(first) {
		return nil, nil, false
	}
	for i := 0; i < len(rest); {
		line, _, more := bytes.Cut(rest[i:], []byte("\n"))
		end := i + len(line)
		if more {
			end++
		}
		if isFence(line) {
			return rest[:i], rest[end:], true
		}
		i = end
	}
	return nil, nil, false
}

func isFence(line []byte) bool { return strings.TrimRight(string(line), " \t\r") == "---" }

type converter struct{ errs []*Error }

func (c *converter) errorf(n *yaml.Node, format string, args ...any) {
	c.errs = append(c.errs, &Error{n.Line, fmt.Sprintf(format, args...)})
}

// value turns a node into the values JSON has. Dates stay as written.
func (c *converter) value(n *yaml.Node) any {
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		c.errorf(n, "YAML anchors and aliases (& and *) aren't supported; quote the value if it's text")
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode {
				c.errorf(key, "a field name must be plain text")
				continue
			}
			if key.Value == "<<" && key.Tag == "!!merge" {
				c.errorf(key, "YAML merge keys (<<) aren't supported; write the fields out")
				continue
			}
			if _, dup := m[key.Value]; dup {
				c.errorf(key, "`%s` appears twice", key.Value)
				continue
			}
			m[key.Value] = c.value(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			s = append(s, c.value(item))
		}
		return s
	}
	switch n.Tag {
	case "!!str", "!!timestamp":
		return n.Value
	case "!!int", "!!float", "!!bool", "!!null":
		var v any
		if err := n.Decode(&v); err != nil {
			c.errorf(n, "%v", err)
		}
		if f, ok := v.(float64); ok && (math.IsInf(f, 0) || math.IsNaN(f)) {
			c.errorf(n, "`%s` isn't a number a record can hold; put it in quotes if it's meant as text", n.Value)
			return nil
		}
		return v
	}
	c.errorf(n, "YAML tags such as %s aren't supported", n.Tag)
	return nil
}

// Line finds the line of the value at a path, or of the nearest thing
// around it that's there. A field's line is the line its name is on.
func (r *Record) Line(p Path) int {
	n, line := r.Header.node, 1
	for _, part := range p {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			if key, value := (Map{node: n}).find(part); key != nil {
				next, line = value, key.Line
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(part); err == nil && i >= 0 && i < len(n.Content) {
				next, line = n.Content[i], n.Content[i].Line
			}
		}
		if next == nil {
			break
		}
		n = next
	}
	return line
}
