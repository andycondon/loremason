package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
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
	typ, err := registry.Load().Find(names[0])
	if err != nil {
		fmt.Fprintf(stderr, "mason: %v\n", err)
		return 2
	}
	if *title == "" {
		return misuse(stderr, "new needs --title")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	rel, err := graph.Create(w.Root, typ, *title, time.Now())
	if errors.Is(err, graph.ErrTitle) {
		return misuse(stderr, "%v", err)
	}
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, filepath.Join(w.Root, filepath.FromSlash(rel)))
	return 0
}
