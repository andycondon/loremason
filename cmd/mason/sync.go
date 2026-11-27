package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/page"
	"github.com/andycondon/loremason/internal/pinning"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/vcs"
)

var syncCmd = &cli.Command{
	Name:    "sync",
	Summary: "pin links and record fingerprints",
	Setup: func(fs *flag.FlagSet) cli.Action {
		dryRun := fs.Bool("dry-run", false, "say what would change, and change nothing")
		return func(env *cli.Env, _ []string) error {
			w, err := open(env)
			if err != nil {
				return err
			}
			res, x, err := load(w.Root)
			if err != nil {
				return err
			}
			defer x.Close()
			plan := pinning.Sync(res.Graph, res.Broken(), today())
			pages, _, err := page.Plan(w.Root, x)
			if err != nil {
				return err
			}
			printWarnings(env, plan)
			for _, path := range plan.Skipped {
				fmt.Fprintf(env.Stdout, "%s: skipped, since it has errors; see `mason validate`\n", path)
			}
			for _, s := range pages.Skipped {
				fmt.Fprintln(env.Stdout, s)
			}
			verb := ""
			if *dryRun {
				verb = "would "
			}
			for _, a := range plan.Actions {
				fmt.Fprintf(env.Stdout, "%s: %s%s\n", a.Path, verb, a.What)
			}
			for _, a := range pages.Actions {
				fmt.Fprintf(env.Stdout, "%s: %s%s\n", a.Path, verb, a.What)
			}
			if !*dryRun {
				if err := pinning.Apply(w.Root, plan); err != nil {
					return err
				}
				if err := pages.Apply(w.Root); err != nil {
					return err
				}
			}
			skipped := len(plan.Skipped) > 0 || len(pages.Skipped) > 0
			if plan.Empty() && pages.Empty() && !skipped {
				fmt.Fprintln(env.Stdout, "nothing to sync")
			}
			if skipped {
				return cli.ErrFailed
			}
			return nil
		}
	},
}

var reviseCmd = &cli.Command{
	Name:    "revise",
	Summary: "open a new revision before changing a record's meaning",
	Args:    []string{"record ID"},
	Setup: func(fs *flag.FlagSet) cli.Action {
		breaking := fs.Bool("breaking", false, "the new revision removes or narrows something")
		nonBreaking := fs.Bool("non-breaking", false, "the new revision only adds or clarifies")
		return func(env *cli.Env, args []string) error {
			if *breaking == *nonBreaking {
				return cli.Usagef("say whether the change is --breaking (it removes or narrows something) or --non-breaking (it only adds or clarifies)")
			}
			w, err := open(env)
			if err != nil {
				return err
			}
			res, err := check.Run(w.Root, registry.Load())
			if err != nil {
				return err
			}
			n, err := res.Graph.Lookup(args[0])
			if err != nil {
				return err
			}
			plan, err := pinning.Revise(n, *breaking, res.Broken(), today(), vcs.Commit(w.Root, n.File.Path))
			if err != nil {
				return err
			}
			printWarnings(env, plan)
			if err := pinning.Apply(w.Root, plan); err != nil {
				return err
			}
			for _, a := range plan.Actions {
				fmt.Fprintln(env.Stdout, a)
			}
			return nil
		}
	},
}

func today() string { return time.Now().Format(time.DateOnly) }

func printWarnings(env *cli.Env, p *pinning.Plan) {
	for _, w := range p.Warnings {
		env.Warnf("%s", w)
	}
}
