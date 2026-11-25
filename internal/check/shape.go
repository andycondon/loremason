package check

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/suggest"
)

var printer = message.NewPrinter(language.English)

// shape checks a header against its type's JSON Schema.
func (c *checker) shape(f *graph.File) {
	if f.Record == nil || f.Record.Data == nil || f.Type != f.Folder {
		return // the reader, or the check of the path, has already said why
	}
	var invalid *jsonschema.ValidationError
	if errors.As(f.Type.Schema.Validate(f.Record.Data), &invalid) {
		c.leaves(f, invalid)
	}
}

// leaves reports the innermost causes; the outer ones only say "see below".
func (c *checker) leaves(f *graph.File, e *jsonschema.ValidationError) {
	if len(e.Causes) > 0 {
		for _, cause := range e.Causes {
			c.leaves(f, cause)
		}
		return
	}
	loc := e.InstanceLocation
	line := f.Record.Line(loc...)
	if f.Bad[line] {
		return // the value the reader rejected
	}
	at := fieldPath(loc)
	in := "" // where a missing or unknown field is, when it's inside an entry
	if at != "" {
		in = fmt.Sprintf(" in `%s`", at)
	}
	var description string
	sub := c.reg.SubschemaAt(e.SchemaURL)
	if sub != nil {
		description = sub.Description
	}

	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		names := make([]string, len(k.Missing))
		for i, name := range k.Missing {
			names[i] = "`" + name + "`"
		}
		verb := "is"
		if len(names) > 1 {
			verb = "are"
		}
		c.ds.Errorf(f.Path, line, "%s %s missing%s", list(names), verb, in)
	case *kind.AdditionalProperties:
		var known []string
		if sub != nil {
			known = slices.Sorted(maps.Keys(sub.Properties))
		}
		for _, name := range k.Properties {
			line := f.Record.Line(append(slices.Clone(loc), name)...)
			if at == "" && name == "type" {
				c.ds.Errorf(f.Path, line, "unknown field `type`; the type comes from the ID's prefix, so remove it")
				continue
			}
			hint := suggest.Hint(suggest.Near(name, known))
			if hint == "" && at == "" {
				hint = "; fields the schema doesn't know go under `extra`"
			} else if hint == "" {
				hint = "; known fields are " + strings.Join(known, ", ")
			}
			c.ds.Errorf(f.Path, line, "unknown field `%s`%s%s", name, in, hint)
		}
	case *kind.Enum:
		var want []string
		for _, w := range k.Want {
			want = append(want, fmt.Sprint(w))
		}
		msg := fmt.Sprintf("`%s` must be one of %s", at, strings.Join(want, ", "))
		if k.Got == nil {
			msg = fmt.Sprintf("`%s` is empty; it must be one of %s", at, strings.Join(want, ", "))
		} else if got, ok := k.Got.(string); ok {
			msg = fmt.Sprintf("`%s` is `%s`; it must be one of %s", at, got, strings.Join(want, ", "))
			msg += suggest.Hint(suggest.Near(got, want))
		}
		c.ds.Errorf(f.Path, line, "%s", msg)
	case *kind.Type:
		want := description
		if want == "" {
			var names []string
			for _, w := range k.Want {
				names = append(names, typeNames[w])
			}
			want = strings.Join(names, " or ")
		}
		switch k.Got {
		case "null":
			c.ds.Errorf(f.Path, line, "`%s` is empty; it must be %s", at, want)
		case "number", "boolean":
			hint := ""
			if slices.Contains(k.Want, "string") {
				hint = "; put it in quotes if it's meant as text"
			}
			c.ds.Errorf(f.Path, line, "`%s` is a %s; it must be %s%s", at, k.Got, want, hint)
		default:
			got := typeNames[k.Got]
			c.ds.Errorf(f.Path, line, "`%s` is %s; it must be %s", at, got, want)
		}
	case *kind.Format:
		c.ds.Errorf(f.Path, line, "`%s` is %q, but %v", at, k.Got, k.Err)
	case *kind.Pattern:
		switch {
		case k.Got == "":
			c.ds.Errorf(f.Path, line, "`%s` is empty; it must be %s", at, description)
		case description != "":
			c.ds.Errorf(f.Path, line, "`%s` is %q; it must be %s", at, k.Got, description)
		default:
			c.ds.Errorf(f.Path, line, "`%s` is %q, which doesn't match %s", at, k.Got, k.Want)
		}
	default:
		where := ""
		if at != "" {
			where = fmt.Sprintf("`%s`: ", at)
		}
		c.ds.Errorf(f.Path, line, "%s%s", where, e.ErrorKind.LocalizedString(printer))
	}
}

var typeNames = map[string]string{
	"array": "a list", "object": "a map", "string": "text", "integer": "a whole number",
	"number": "a number", "boolean": "true or false", "null": "empty",
}

// list joins names as "a, b and c".
func list(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// fieldPath writes a location as support[0].direction.
func fieldPath(loc []string) string {
	var b strings.Builder
	for _, p := range loc {
		if _, err := strconv.Atoi(p); err == nil {
			fmt.Fprintf(&b, "[%s]", p)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(p)
	}
	return b.String()
}
