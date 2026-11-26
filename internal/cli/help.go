package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// help writes the app's help: its summary, how to call it, its commands and
// its own flags.
func (a *App) help(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(w, "%s\n\nUsage:\n  %s <command> [arguments]\n\nCommands:\n", a.Summary, a.Name)
	tw := table(w)
	for _, c := range a.Commands {
		fmt.Fprintf(tw, "  %s\t%s\n", c.Name, c.Summary)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nRun `%s <command> --help` for a command's arguments and flags.\n", a.Name)
	if a.Footer != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimRight(a.Footer, "\n"))
	}
	writeFlags(w, fs)
}

// help writes a command's help: how to call it, what it does, and its flags.
func (c *Command) help(a *App, w io.Writer, fs *flag.FlagSet) {
	call := a.Name + " " + c.Name
	for _, arg := range c.Args {
		call += " <" + arg + ">"
	}
	fmt.Fprintf(w, "Usage:\n  %s [flags]\n\n%s\n", call, c.Summary)
	writeFlags(w, fs)
}

// writeFlags lists a flag set's flags, and --help, which every flag set has.
// A flag that takes a value shows the value's name, taken from a word in
// backquotes in its usage, as package flag does.
func writeFlags(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, "\nFlags:\n")
	tw := table(w)
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			name = " <" + name + ">"
		}
		fmt.Fprintf(tw, "  --%s%s\t%s\n", f.Name, name, usage)
	})
	fmt.Fprint(tw, "  --help\tshow this help\n")
	tw.Flush()
}

func table(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 0, 3, ' ', 0) }
