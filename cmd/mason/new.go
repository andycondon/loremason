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

// attached are the types `new` attaches to the session's activity: the two
// with no other path back to their initiative.
var attached = map[string]bool{"finding": true, "evidence": true}

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
				return cli.Usagef("%v", err) // the environment is wrong, as a flag would be
			}
			var opts graph.CreateOptions
			attach := attached[typ.Name]
			now := time.Now()
			// Check the title first, so a bad one leaves no activity behind.
			clean, err := graph.CleanTitle(*title)
			if err != nil {
				return cli.Usagef("%v", err)
			}
			switch {
			case attach && sess != nil:
				sess.Warn = env.Stderr
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
			rel, err := graph.Create(w.Root, typ, clean, now, opts)
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
