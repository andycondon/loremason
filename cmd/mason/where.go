package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/index"
)

var whereCmd = &cli.Command{
	Name:    "where",
	Summary: "show where an initiative stands",
	Args:    []string{"initiative"},
	Setup: func(*flag.FlagSet) cli.Action {
		return query(func(x *index.Index, args []string, w io.Writer) error {
			v, err := x.Where(args[0])
			if err != nil {
				return err
			}
			printView(w, v)
			return nil
		})
	},
}

// printView prints the view: the initiative, then its four sections. A
// section with nothing in it says "none". Records print with line, and what
// belongs to a record is indented under it.
func printView(w io.Writer, v *index.View) {
	fmt.Fprintln(w, line(v.Initiative))

	section(w, "in progress", len(v.InProgress), func() {
		for _, s := range v.InProgress {
			fmt.Fprintf(w, "  %s\n", line(s))
		}
	})
	section(w, "blocked", len(v.Blocked), func() {
		for _, b := range v.Blocked {
			fmt.Fprintf(w, "  %s\n", line(b.Task))
			for _, by := range b.By {
				fmt.Fprintf(w, "    waiting on  %s\n", line(by))
			}
		}
	})
	section(w, "open questions", len(v.Open), func() {
		for _, q := range v.Open {
			fmt.Fprintf(w, "  %s\n", line(q.Summary))
			for _, p := range q.Pending {
				fmt.Fprintf(w, "    answer waiting  %s\n", line(p))
			}
		}
	})
	section(w, "needs a re-check", len(v.Recheck), func() {
		for _, c := range v.Recheck {
			fmt.Fprintf(w, "  %s  r%d\n", line(c.Target), c.Revision)
			for _, l := range c.Links {
				var why []string
				if l.Stale {
					why = append(why, "stale")
				}
				if l.Reopens {
					why = append(why, "reopens")
				}
				fmt.Fprintf(w, "    %s  %s  (%s)\n", l.Field, linkLine(l.Link), strings.Join(why, ", "))
			}
		}
	})
}

// section prints a heading, then what fill prints, or "none" when n is zero.
func section(w io.Writer, heading string, n int, fill func()) {
	fmt.Fprintf(w, "\n%s\n", heading)
	if n == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	fill()
}
