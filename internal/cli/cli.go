// Package cli implements the mason command.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/workspace"
)

// Version is the loremason version. Release builds may override it with
// -ldflags "-X github.com/andycondon/loremason/internal/cli.Version=…".
var Version = "0.0.1"

const usage = `mason stores, checks and queries a context graph of typed, linked records.

Usage:
  mason [flags] <command>

Commands:
  validate                     check every record in the workspace
  new <type> --title <title>   create a record and print its path

The workspace is the nearest folder, here or above, with a .loremason folder.
Set LOREMASON_WORKSPACE to use another.

Flags:
  --version   print the version and exit
  --help      show this help
`

// Run runs mason with the given arguments (without the program name) and
// returns the process exit code: 0 for success, 1 when records have errors or
// a command fails, 2 for a mistake in the arguments.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := newFlags(stderr)
	version := fs.Bool("version", false, "print the version and exit")
	if code, done := parse(fs, args); done {
		return code
	}
	if *version {
		fmt.Fprintf(stdout, "mason %s\n", Version)
		return 0
	}
	if fs.NArg() == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}
	name, rest := fs.Arg(0), fs.Args()[1:]
	switch name {
	case "validate":
		return runValidate(rest, stdout, stderr)
	case "new":
		return runNew(rest, stdout, stderr)
	}
	return misuse(stderr, "unknown command `%s`", name)
}

func newFlags(stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("mason", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	return fs
}

// parse parses flags. done is true when the command should stop with code.
func parse(fs *flag.FlagSet, args []string) (code int, done bool) {
	switch err := fs.Parse(args); err {
	case nil:
		return 0, false
	case flag.ErrHelp:
		return 0, true
	default:
		return 2, true
	}
}

func misuse(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "mason: "+format+"; see mason --help\n", args...)
	return 2
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "mason: %v\n", err)
	return 1
}

// open finds the workspace and says so when it's pinned to another version.
func open(stderr io.Writer) (*workspace.Workspace, error) {
	w, err := workspace.Find(".", os.Getenv(workspace.Env))
	if err == nil && w.Config.Version != Version {
		fmt.Fprintf(stderr, "mason: warning: this is mason %s, but the workspace uses %s\n", Version, w.Config.Version)
	}
	return w, err
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlags(stderr)
	if code, done := parse(fs, args); done {
		return code
	}
	if fs.NArg() > 0 {
		return misuse(stderr, "validate takes no arguments")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		return fail(stderr, err)
	}
	for _, p := range res.Problems {
		fmt.Fprintln(stdout, p)
	}
	errs, warnings := res.Count()
	fmt.Fprintf(stdout, "%s: %s, %s\n", plural(res.Records, "record"), plural(errs, "error"), plural(warnings, "warning"))
	if errs > 0 {
		return 1
	}
	return 0
}

func plural(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}
