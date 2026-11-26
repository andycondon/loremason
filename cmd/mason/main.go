// Command mason creates and checks the records of a loremason context graph,
// and keeps the links between them pinned.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/workspace"
)

// Version is the loremason version. Release builds may override it with
// -ldflags "-X main.Version=…".
var Version = "0.0.1"

var app = &cli.App{
	Name:    "mason",
	Version: Version,
	Summary: "mason creates and checks the records of a context graph, and keeps the links\nbetween them pinned.",
	Footer: "The workspace is the nearest folder, here or above, with a .loremason folder.\n" +
		"Set " + workspace.Env + " to use another.",
	Commands: []*cli.Command{validateCmd, newCmd, syncCmd, reviseCmd},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run runs mason with the given arguments (without the program name) and
// returns the exit code: 0 for success, 1 when records have errors or a
// command fails, 2 for a mistake in the arguments.
func run(args []string, stdout, stderr io.Writer) int {
	return app.Run(args, stdout, stderr)
}

// open finds the workspace, and warns when it uses another version of
// loremason.
func open(env *cli.Env) (*workspace.Workspace, error) {
	w, err := workspace.Find(".", os.Getenv(workspace.Env))
	if err != nil {
		return nil, err
	}
	if err := w.CheckVersion(Version); err != nil {
		env.Warnf("%v", err)
	}
	return w, nil
}

func plural(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}
