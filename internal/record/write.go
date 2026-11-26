package record

import (
	"bytes"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Out is a YAML value to write into a header. Tools write only the fields
// they own (link lists and revisions), and always in one form: block lists,
// flow maps for links, and block maps for revisions.
type Out struct{ node *yaml.Node }

// KV is one field of a map being written.
type KV struct {
	Key string
	Val Out
}

func scalar(tag, value string, style yaml.Style) Out {
	return Out{&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value, Style: style}}
}

// Str writes text, quoted only when YAML needs it.
func Str(s string) Out { return scalar("!!str", s, 0) }

// Quoted writes text in double quotes, as fingerprints are written.
func Quoted(s string) Out { return scalar("!!str", s, yaml.DoubleQuotedStyle) }

// Int writes a whole number.
func Int(i int) Out { return scalar("!!int", strconv.Itoa(i), 0) }

// Date writes a date such as 2026-11-22 without quotes.
func Date(s string) Out { return scalar("!!timestamp", s, 0) }

// List writes a block list.
func List(items ...Out) Out {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range items {
		n.Content = append(n.Content, item.node)
	}
	return Out{n}
}

// BlockMap writes a map with a field per line, in the order given.
func BlockMap(fields ...KV) Out { return Out{mapping(fields, 0)} }

// FlowMap writes a map on one line: {to: bel_7k3m9q, rev: 1}.
func FlowMap(fields ...KV) Out { return Out{mapping(fields, yaml.FlowStyle)} }

func mapping(fields []KV, style yaml.Style) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Style: style}
	for _, f := range fields {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: f.Key}, f.Val.node)
	}
	return n
}

// Out writes a value again as it was read, comments included.
func (v Value) Out() Out { return Out{v.node} }

// WithCommentsOf carries the comments before and beside v over to o, so a
// comment by a link survives the link being pinned. Comment lines after v
// aren't carried; they stay in the file where they are.
func (o Out) WithCommentsOf(v Value) Out {
	if v.node == nil {
		return o
	}
	n := *o.node
	n.HeadComment, n.LineComment = v.node.HeadComment, v.node.LineComment
	return Out{&n}
}

// WithoutComments returns the value with no comments, for keeping old
// wording without the notes that were beside it.
func (o Out) WithoutComments() Out {
	return Out{withoutComments(o.node)}
}

func withoutComments(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.HeadComment, c.LineComment, c.FootComment = "", "", ""
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = withoutComments(child)
	}
	return &c
}

// Plain returns the value as plain data, as fingerprints see it.
func (o Out) Plain() any {
	var c converter
	return c.value(o.node)
}

// Render writes a new field and its value as header lines, ending in a
// newline.
func Render(field string, value Out) string {
	return render(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, value)
}

// Rewrite renders the field that holds v with a new value. Comments on the
// field's own line are kept: YAML attaches them to the field's name, or, for
// a one-line list or value, to the value.
func (v Value) Rewrite(value Out) string {
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v.Path[len(v.Path)-1]}
	if v.key != nil {
		key.HeadComment, key.LineComment, key.FootComment = v.key.HeadComment, v.key.LineComment, v.key.FootComment
	}
	if v.node != nil && v.node.Kind == yaml.SequenceNode && key.LineComment == "" {
		key.LineComment = v.node.LineComment
	}
	return render(key, value)
}

func render(key *yaml.Node, value Out) string {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, value.node}}); err != nil {
		panic("record: " + err.Error()) // nodes built here always encode
	}
	return b.String()
}

// Quote returns s as YAML text, quoted only when YAML needs it.
func Quote(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(Render("x", Str(s)), "x: "), "\n")
}
