package main

import (
	"flag"
	"fmt"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/registry"
)

var validateCmd = &cli.Command{
	Name:    "validate",
	Summary: "check every record in the workspace",
	Setup:   func(*flag.FlagSet) cli.Action { return validate },
}

// validate prints what the check found, and fails when it found errors.
func validate(env *cli.Env, _ []string) error {
	w, err := open(env)
	if err != nil {
		return err
	}
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		return err
	}
	for _, d := range res.Diagnostics {
		fmt.Fprintln(env.Stdout, d)
	}
	errs, warnings := res.Diagnostics.Count()
	fmt.Fprintf(env.Stdout, "%s: %s, %s\n", plural(res.Records, "record"), plural(errs, "error"), plural(warnings, "warning"))
	if errs > 0 {
		return cli.ErrFailed
	}
	return nil
}
