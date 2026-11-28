package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/registry"
)

var findCmd = &cli.Command{
	Name:     "find",
	Summary:  "list the records whose title or body has all of the words",
	Args:     []string{"words"},
	Variadic: true,
	Setup: func(*flag.FlagSet) cli.Action {
		search := query(func(x *index.Index, args []string, w io.Writer) error {
			found, err := x.Find(strings.Join(args, " "))
			for _, s := range found {
				fmt.Fprintln(w, line(s))
			}
			return err
		})
		return func(env *cli.Env, args []string) error {
			if len(index.Words(strings.Join(args, " "))) == 0 {
				env.Warnf("no words to search for; find looks for letters and digits")
				return nil
			}
			return search(env, args)
		}
	},
}

var showCmd = &cli.Command{
	Name:    "show",
	Summary: "print a record's text and its links in both directions",
	Args:    []string{"record ID"},
	Setup: func(*flag.FlagSet) cli.Action {
		return query(func(x *index.Index, args []string, w io.Writer) error {
			d, err := x.Show(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(w, line(d.Summary))
			if body := strings.TrimSpace(d.Body); body != "" {
				fmt.Fprintf(w, "\n%s\n", body)
			}
			printLinks(w, "links out:", d.Out)
			printLinks(w, "links in:", d.In)
			return nil
		})
	},
}

var dependentsCmd = &cli.Command{
	Name:    "dependents",
	Summary: "list the records that rely on a record, directly or through others",
	Args:    []string{"record ID"},
	Setup: func(*flag.FlagSet) cli.Action {
		return query(func(x *index.Index, args []string, w io.Writer) error {
			deps, err := x.Dependents(args[0])
			for _, d := range deps {
				fmt.Fprintf(w, "%d  %s  via %s of %s\n", d.Depth, line(d.Summary), d.Field, d.On.ID)
			}
			return err
		})
	},
}

var staleCmd = &cli.Command{
	Name:    "stale",
	Summary: "list the records that changed since links were pinned to them, and links to closed records",
	Setup: func(*flag.FlagSet) cli.Action {
		return query(func(x *index.Index, _ []string, w io.Writer) error {
			groups, err := x.Stale()
			for _, g := range groups {
				fmt.Fprintf(w, "%s  r%d\n", line(g.Target), g.Revision)
				for _, l := range g.Links {
					fmt.Fprintf(w, "  %s  %s%s\n", l.Field, linkLine(l.Link), orphanedNote(l.Orphaned, g.Target))
				}
			}
			return err
		})
	},
}

// orphanedNote is the reason a link to a record that is no longer in force
// needs a look, as a suffix for its line, or "" when the link isn't one.
func orphanedNote(orphaned bool, target index.Summary) string {
	if !orphaned {
		return ""
	}
	return "  (" + orphanedReason(target) + ")"
}

// orphanedReason says what a link rests on.
func orphanedReason(target index.Summary) string {
	return fmt.Sprintf("rests on %s, which is %s", target.ID, target.Status)
}

var reopenedCmd = &cli.Command{
	Name:    "reopened",
	Summary: "list the commitments resting on a belief that has fallen",
	Setup: func(*flag.FlagSet) cli.Action {
		return query(func(x *index.Index, _ []string, w io.Writer) error {
			rs, err := x.Reopened()
			for _, r := range rs {
				fmt.Fprintf(w, "%s  rests on  %s  %s  (%s)\n", line(r.Commitment), r.Belief.ID, r.Belief.Title, r.Status)
			}
			return err
		})
	},
}

// query makes an action that opens the index, warns when the graph has
// errors, and runs one query that prints to stdout.
func query(run func(x *index.Index, args []string, stdout io.Writer) error) cli.Action {
	return func(env *cli.Env, args []string) error {
		w, err := open(env)
		if err != nil {
			return err
		}
		x, err := index.Open(w.Root, registry.Load())
		if err != nil {
			return err
		}
		defer x.Close()
		if n := x.Problems(); n > 0 {
			env.Warnf("the graph has %s; run mason validate", plural(n, "error"))
		}
		return run(x, args, env.Stdout)
	}
}

// line is the one format for a record in a list: ID, type, status and title,
// two spaces apart. A record with no status shows "-".
func line(s index.Summary) string {
	status := s.Status
	if status == "" {
		status = "-"
	}
	return fmt.Sprintf("%s  %s  %s  %s", s.ID, s.Type, status, s.Title)
}

// linkLine is the record at the other end of a link, with its pin if any.
func linkLine(l index.Link) string {
	s := line(l.Other)
	if l.Pin != nil {
		s += fmt.Sprintf("  pinned r%d", l.Pin.Rev)
		if l.Pin.FP != "" {
			s += " " + l.Pin.FP
		}
	}
	return s
}

func printLinks(w io.Writer, heading string, links []index.Link) {
	if len(links) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", heading)
	for _, l := range links {
		field := l.Field
		if l.Local != "" {
			field += " [" + l.Local + "]" // made by that sub-record
		}
		fmt.Fprintf(w, "  %s  %s\n", field, linkLine(l))
	}
}
