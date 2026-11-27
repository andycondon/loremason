// Command mason creates and checks the records of a loremason context graph,
// keeps the links between them pinned, and answers questions about them.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/cli"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/workspace"
)

// Version is the loremason version. Release builds may override it with
// -ldflags "-X main.Version=…".
var Version = "0.0.1"

var app = &cli.App{
	Name:    "mason",
	Version: Version,
	Summary: "mason creates and checks the records of a context graph, keeps the links\nbetween them pinned, and answers questions about them.",
	Footer: "The workspace is the nearest folder, here or above, with a .loremason folder.\n" +
		"Set " + workspace.Env + " to use another.",
	Commands: []*cli.Command{validateCmd, newCmd, syncCmd, reviseCmd, findCmd, showCmd, dependentsCmd, staleCmd, reopenedCmd, whereCmd},
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

// load checks the workspace and opens the index over the result, for a
// command that needs both the check result and the index, so the graph is
// loaded once. The signature is taken before the load, so a file edited while
// the command runs makes the next open rebuild. The caller closes the index.
func load(root string) (*check.Result, *index.Index, error) {
	sig, err := index.Signature(root)
	if err != nil {
		return nil, nil, err
	}
	res, err := check.Run(root, registry.Load())
	if err != nil {
		return nil, nil, err
	}
	x, err := index.FromCheck(root, res, sig)
	if err != nil {
		return nil, nil, err
	}
	return res, x, nil
}
