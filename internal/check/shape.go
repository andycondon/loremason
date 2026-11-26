package check

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/suggest"
)

// shape checks a header against its type's JSON Schema, and turns each
// failure into a message in plain words at the right line.
func (c *checker) shape(f *graph.File) {
	if f.Record == nil || f.Record.Data == nil || f.Misplaced() {
		return // reading the file, or placement, has already said why
	}
	var invalid *jsonschema.ValidationError
	if errors.As(f.Type.Validate(f.Record.Data), &invalid) {
		for _, leaf := range leaves(invalid) {
			c.schemaError(f, leaf)
		}
	}
}

// leaves returns the innermost causes of a failure. The ones around them
// only say that something inside failed.
func leaves(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	var out []*jsonschema.ValidationError
	for _, cause := range e.Causes {
		out = append(out, leaves(cause)...)
	}
	return out
}

// failure is one schema failure, located in the file.
type failure struct {
	at  record.Path
	pos diag.Pos
	// description is the schema's own words for what's wanted, such as
	// "a date such as 2026-11-22"; empty when it has none.
	description string
	schema      *jsonschema.Schema // the part of the schema that failed; may be nil
}

func (c *checker) schemaError(f *graph.File, e *jsonschema.ValidationError) {
	fl := failure{at: record.Path(e.InstanceLocation)}
	fl.pos = f.At(f.Node.Fields.LineOf(fl.at))
	if f.Rejected(fl.pos.Line) {
		return // the reader has already reported this value
	}
	if fl.schema = c.reg.SubschemaAt(e.SchemaURL); fl.schema != nil {
		fl.description = fl.schema.Description
	}
	if k, ok := e.ErrorKind.(*kind.AdditionalProperties); ok {
		for _, name := range k.Properties {
			c.ds.Errorf(f.At(f.Node.Fields.LineOf(fl.at.Field(name))), "%s", unknownField(fl, name))
		}
		return
	}
	c.ds.Errorf(fl.pos, "%s", explain(fl, e.ErrorKind))
}

// unknownField explains a field the schema doesn't have.
func unknownField(fl failure, name string) string {
	topLevel := len(fl.at) == 0
	if topLevel && name == "type" {
		return "unknown field `type`; the type comes from the ID's prefix, so remove it"
	}
	var known []string
	if fl.schema != nil {
		known = slices.Sorted(maps.Keys(fl.schema.Properties))
	}
	where := ""
	if !topLevel {
		where = fmt.Sprintf(" in `%s`", fl.at)
	}
	hint := suggest.Hint(suggest.Near(name, known))
	switch {
	case hint != "":
	case topLevel:
		hint = "; fields the schema doesn't know go under `extra`"
	default:
		hint = "; known fields are " + strings.Join(known, ", ")
	}
	return fmt.Sprintf("unknown field `%s`%s%s", name, where, hint)
}

// explain explains every other kind of failure.
func explain(fl failure, k jsonschema.ErrorKind) string {
	switch k := k.(type) {
	case *kind.Required:
		return missing(fl, k.Missing)
	case *kind.Enum:
		return notAllowed(fl, k)
	case *kind.Type:
		return wrongType(fl, k)
	case *kind.Format:
		return fmt.Sprintf("`%s` is %q, but %v", fl.at, k.Got, k.Err)
	case *kind.Pattern:
		switch {
		case k.Got == "":
			return fmt.Sprintf("`%s` is empty; it must be %s", fl.at, fl.description)
		case fl.description != "":
			return fmt.Sprintf("`%s` is %q; it must be %s", fl.at, k.Got, fl.description)
		default:
			return fmt.Sprintf("`%s` is %q, which doesn't match %s", fl.at, k.Got, k.Want)
		}
	}
	// Kinds the schemas don't use today get the library's own words.
	if len(fl.at) == 0 {
		return k.LocalizedString(printer)
	}
	return fmt.Sprintf("`%s`: %s", fl.at, k.LocalizedString(printer))
}

var printer = message.NewPrinter(language.English)

func missing(fl failure, names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "`" + name + "`"
	}
	verb := "is"
	if len(names) > 1 {
		verb = "are"
	}
	where := ""
	if len(fl.at) > 0 {
		where = fmt.Sprintf(" in `%s`", fl.at)
	}
	return fmt.Sprintf("%s %s missing%s", andList(quoted), verb, where)
}

func notAllowed(fl failure, k *kind.Enum) string {
	var want []string
	for _, w := range k.Want {
		want = append(want, fmt.Sprint(w))
	}
	allowed := strings.Join(want, ", ")
	switch got := k.Got.(type) {
	case nil:
		return fmt.Sprintf("`%s` is empty; it must be one of %s", fl.at, allowed)
	case string:
		return fmt.Sprintf("`%s` is `%s`; it must be one of %s%s", fl.at, got, allowed, suggest.Hint(suggest.Near(got, want)))
	default:
		return fmt.Sprintf("`%s` must be one of %s", fl.at, allowed)
	}
}

func wrongType(fl failure, k *kind.Type) string {
	want := fl.description
	if want == "" {
		var names []string
		for _, w := range k.Want {
			names = append(names, typeNames[w])
		}
		want = strings.Join(names, " or ")
	}
	switch k.Got {
	case "null":
		return fmt.Sprintf("`%s` is empty; it must be %s", fl.at, want)
	case "number", "boolean":
		hint := ""
		if slices.Contains(k.Want, "string") {
			hint = "; put it in quotes if it's meant as text"
		}
		return fmt.Sprintf("`%s` is a %s; it must be %s%s", fl.at, k.Got, want, hint)
	default:
		return fmt.Sprintf("`%s` is %s; it must be %s", fl.at, typeNames[k.Got], want)
	}
}
