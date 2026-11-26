package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// runNew is `mason new <type> --title …`. It prints the new record's path.
func runNew(args []string, stdout, stderr io.Writer) int {
	flags := newFlags(stderr)
	title := flags.String("title", "", "the record's title")
	// Flags may come before or after the type.
	var names []string
	for rest := args; ; {
		if code, done := parse(flags, rest); done {
			return code
		}
		if flags.NArg() == 0 {
			break
		}
		names, rest = append(names, flags.Arg(0)), flags.Args()[1:]
	}
	if len(names) != 1 {
		return misuse(stderr, "new takes one record type (quote a title that has spaces)")
	}
	reg := registry.Load()
	typ := reg.Lookup(names[0])
	if typ == nil {
		hint := "the types are " + strings.Join(reg.Names(), ", ")
		if s := suggest.Near(names[0], reg.Names()); s != "" {
			hint = fmt.Sprintf("did you mean `%s`?", s)
		}
		fmt.Fprintf(stderr, "mason: unknown record type `%s`; %s\n", names[0], hint)
		return 2
	}
	if !utf8.ValidString(*title) {
		return misuse(stderr, "the title isn't valid UTF-8")
	}
	if strings.TrimSpace(*title) == "" || strings.ContainsFunc(*title, unicode.IsControl) {
		return misuse(stderr, "new needs --title with one line of text")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	rel, err := graph.Create(w.Root, typ, strings.TrimSpace(*title), time.Now())
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, filepath.Join(w.Root, filepath.FromSlash(rel)))
	return 0
}
