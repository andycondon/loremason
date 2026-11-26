package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
)

var newCmd = &cli.Command{
	Name:    "new",
	Summary: "create a record and print its path",
	Args:    []string{"record type"},
	Setup: func(fs *flag.FlagSet) cli.Action {
		title := fs.String("title", "", "the record's `title`")
		return func(env *cli.Env, args []string) error {
			typ, err := registry.Load().Find(args[0])
			if err != nil {
				return cli.Usagef("%v", err)
			}
			if *title == "" {
				return cli.Usagef("new needs --title")
			}
			w, err := open(env)
			if err != nil {
				return err
			}
			rel, err := graph.Create(w.Root, typ, *title, time.Now())
			if errors.Is(err, graph.ErrTitle) {
				return cli.Usagef("%v", err)
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(env.Stdout, filepath.Join(w.Root, filepath.FromSlash(rel)))
			return nil
		}
	},
}
