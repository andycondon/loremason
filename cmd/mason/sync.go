package main

import (
	"fmt"
	"io"
	"time"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/pinning"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/vcs"
)

// runSync is `mason sync [--dry-run]`. It exits 1 when files were skipped
// because they have errors.
func runSync(args []string, stdout, stderr io.Writer) int {
	flags := newFlags(stderr)
	dryRun := flags.Bool("dry-run", false, "say what would change, and change nothing")
	if code, done := parse(flags, args); done {
		return code
	}
	if flags.NArg() > 0 {
		return misuse(stderr, "sync takes no arguments")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	reg := registry.Load()
	g, loaded, err := graph.Load(w.Root, reg)
	if err != nil {
		return fail(stderr, err)
	}
	plan := pinning.Sync(g, check.Graph(g, loaded, reg).Broken(), today())
	printWarnings(stderr, plan)
	for _, path := range plan.Skipped {
		fmt.Fprintf(stdout, "%s: skipped, since it has errors; see `mason validate`\n", path)
	}
	verb := ""
	if *dryRun {
		verb = "would "
	}
	for _, a := range plan.Actions {
		fmt.Fprintf(stdout, "%s: %s%s\n", a.Path, verb, a.What)
	}
	if !*dryRun {
		if err := pinning.Apply(w.Root, plan); err != nil {
			return fail(stderr, err)
		}
	}
	if plan.Empty() {
		fmt.Fprintln(stdout, "nothing to sync")
	}
	if len(plan.Skipped) > 0 {
		return 1
	}
	return 0
}

// runRevise is `mason revise <id> --breaking|--non-breaking`.
func runRevise(args []string, stdout, stderr io.Writer) int {
	flags := newFlags(stderr)
	breaking := flags.Bool("breaking", false, "the new revision removes or narrows something")
	nonBreaking := flags.Bool("non-breaking", false, "the new revision only adds or clarifies")
	ids, code, done := parseArgs(flags, args)
	if done {
		return code
	}
	switch {
	case len(ids) != 1:
		return misuse(stderr, "revise takes one record ID")
	case *breaking == *nonBreaking:
		return misuse(stderr, "say whether the change is --breaking (it removes or narrows something) or --non-breaking (it only adds or clarifies)")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	reg := registry.Load()
	g, loaded, err := graph.Load(w.Root, reg)
	if err != nil {
		return fail(stderr, err)
	}
	n, err := g.Lookup(ids[0])
	if err != nil {
		return fail(stderr, err)
	}
	plan, err := pinning.Revise(n, *breaking, check.Graph(g, loaded, reg).Broken(), today(), vcs.Commit(w.Root, n.File.Path))
	if err != nil {
		return fail(stderr, err)
	}
	printWarnings(stderr, plan)
	if err := pinning.Apply(w.Root, plan); err != nil {
		return fail(stderr, err)
	}
	for _, a := range plan.Actions {
		fmt.Fprintln(stdout, a)
	}
	return 0
}

func today() string { return time.Now().Format(time.DateOnly) }

func printWarnings(stderr io.Writer, p *pinning.Plan) {
	for _, w := range p.Warnings {
		fmt.Fprintf(stderr, "mason: warning: %s\n", w)
	}
}
