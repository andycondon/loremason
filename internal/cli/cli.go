// Package cli implements the mason command.
package cli

import (
	"flag"
	"fmt"
	"io"
)

// Version is the loremason version. Release builds may override it with
// -ldflags "-X github.com/andycondon/loremason/internal/cli.Version=…".
var Version = "0.0.1"

const usage = `mason stores, checks and queries a context graph of typed, linked records.

Usage:
  mason [flags]

Flags:
  --version   print the version and exit
  --help      show this help
`

// Run runs mason with the given arguments (without the program name) and
// returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mason", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	version := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *version {
		fmt.Fprintf(stdout, "mason %s\n", Version)
		return 0
	}
	fmt.Fprint(stdout, usage)
	return 0
}
