// Package splice edits the fields of a record's header that tools own, and
// leaves every other byte of the file exactly as it was written: comments,
// spacing, the order of fields, and the body.
//
// Re-encoding a whole header would change lines nobody touched (comment
// spacing, folded text, quoting), so instead each owned field's lines are
// found and replaced with freshly rendered ones.
package splice

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Header is a record file being edited.
type Header struct {
	bom    string   // a byte order mark the file starts with, kept as it was
	lines  []string // the whole file after any byte order mark, split after each newline
	close  int      // index of the header's closing --- line
	fields map[string]span
	order  []string // fields in file order
	edits  map[string]string
}

// span is the lines a top-level field occupies: from its key to its last
// line of content, not counting blank or comment lines after it, which stay
// where they are.
type span struct{ start, end int }

// Edit reads a record file for editing.
func Edit(data []byte) (*Header, error) {
	h := &Header{fields: map[string]span{}, edits: map[string]string{}}
	text := string(data)
	if rest, ok := strings.CutPrefix(text, "\ufeff"); ok {
		h.bom, text = "\ufeff", rest
	}
	h.lines = strings.SplitAfter(text, "\n")
	if len(h.lines) == 0 || !isFence(h.lines[0]) {
		return nil, errors.New("the file doesn't start with a header")
	}
	h.close = slices.IndexFunc(h.lines[1:], isFence) + 1
	if h.close == 0 {
		return nil, errors.New("the header has no closing ---")
	}
	// A blank line stands in for the opening ---, so YAML lines match the file.
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("\n"+strings.Join(h.lines[1:h.close], "")), &doc); err != nil {
		return nil, fmt.Errorf("the header isn't valid YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the header isn't a list of fields")
	}
	keys := doc.Content[0].Content
	for i := 0; i+1 < len(keys); i += 2 {
		start := keys[i].Line - 1 // lines are 1-based; the slice is 0-based
		end := h.close
		if i+2 < len(keys) {
			end = keys[i+2].Line - 1
		}
		for end > start+1 && isBlankOrComment(h.lines[end-1]) {
			end--
		}
		h.fields[keys[i].Value] = span{start, end}
		h.order = append(h.order, keys[i].Value)
	}
	return h, nil
}

// Set replaces a field's lines with rendered ones, which must start with the
// field's name and end with a newline. A field that isn't there yet goes
// just before `created` and any comments above it, or else at the end of the
// header.
func (h *Header) Set(field, rendered string) {
	h.edits[field] = rendered
}

// Remove deletes a field's lines. A field that isn't there is left alone.
func (h *Header) Remove(field string) {
	h.edits[field] = ""
}

// Bytes returns the edited file.
func (h *Header) Bytes() []byte {
	type change struct {
		start, end int
		text       string
	}
	var changes []change
	for field, text := range h.edits {
		if s, ok := h.fields[field]; ok {
			changes = append(changes, change{s.start, s.end, text})
			continue
		}
		if text == "" {
			continue // removing a field that isn't there
		}
		at := h.close
		if s, ok := h.fields["created"]; ok {
			// Above the comment lines that head created, which belong to it.
			at = s.start
			for at > 1 && strings.HasPrefix(strings.TrimSpace(h.lines[at-1]), "#") {
				at--
			}
		}
		changes = append(changes, change{at, at, text})
	}
	// Apply from the bottom up, so earlier line numbers stay valid. New
	// fields inserted at the same place keep a stable order.
	slices.SortFunc(changes, func(a, b change) int {
		if a.start != b.start {
			return b.start - a.start
		}
		return strings.Compare(b.text, a.text)
	})
	eol := "\n"
	if strings.HasSuffix(h.lines[0], "\r\n") {
		eol = "\r\n" // keep the file's own line endings
	}
	lines := slices.Clone(h.lines)
	for _, c := range changes {
		if c.text == "" {
			lines = slices.Delete(lines, c.start, c.end)
			continue
		}
		text := strings.ReplaceAll(c.text, "\n", eol)
		lines = slices.Replace(lines, c.start, c.end, strings.SplitAfter(strings.TrimSuffix(text, eol), eol)...)
		lines[c.start+strings.Count(strings.TrimSuffix(text, eol), eol)] += eol
	}
	var b bytes.Buffer
	b.WriteString(h.bom)
	for _, l := range lines {
		b.WriteString(l)
	}
	return b.Bytes()
}

func isFence(line string) bool { return strings.TrimRight(line, " \t\r\n") == "---" }

func isBlankOrComment(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "#")
}
