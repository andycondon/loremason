package main

import (
	"flag"
	"fmt"

	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/page"
)

var validateCmd = &cli.Command{
	Name:    "validate",
	Summary: "check every record in the workspace",
	Setup:   func(*flag.FlagSet) cli.Action { return validate },
}

// validate prints what the check found, and what is wrong with the generated
// pages, and fails when it found errors.
func validate(env *cli.Env, _ []string) error {
	w, err := open(env)
	if err != nil {
		return err
	}
	res, x, err := load(w.Root)
	if err != nil {
		return err
	}
	defer x.Close()
	_, pageProblems, err := page.Plan(w.Root, x)
	if err != nil {
		return err
	}
	all := append(diag.List{}, res.Diagnostics...)
	all = append(all, pageProblems...)
	all.Sort()
	for _, d := range all {
		fmt.Fprintln(env.Stdout, d)
	}
	errs, warnings := all.Count()
	fmt.Fprintf(env.Stdout, "%s: %s, %s\n", plural(res.Records, "record"), plural(errs, "error"), plural(warnings, "warning"))
	if errs > 0 {
		return cli.ErrFailed
	}
	return nil
}
