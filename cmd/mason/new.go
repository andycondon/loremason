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
	"github.com/andycondon/loremason/internal/session"
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
			sess, err := session.FromEnv()
			if err != nil {
				return err
			}
			// Findings and evidence have no other path back to their
			// initiative, so they are attached to the session's activity.
			var opts graph.CreateOptions
			attach := typ.Name == "finding" || typ.Name == "evidence"
			now := time.Now()
			switch {
			case attach && sess != nil:
				_, err := sess.Ensure(w.Root, registry.Load(), now)
				if errors.Is(err, session.ErrNoInitiative) {
					return cli.Usagef("%v", err)
				}
				if err != nil {
					return err
				}
				opts.GeneratedBy = sess.ActivityID()
			case attach:
				fmt.Fprintf(env.Stderr, "note: this %s isn't attached to a session; set %s to attach records you write\n", typ.Name, session.EnvSession)
			}
			rel, err := graph.Create(w.Root, typ, *title, now, opts)
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
