// Package cli runs a program made of commands: it parses flags and
// arguments, prints help generated from the commands' definitions, and turns
// a command's error into a message and an exit code.
//
// It knows nothing about loremason, and imports only the standard library
// and package suggest; a test keeps it that way. It holds only what mason
// uses. If mason ever needs nested commands, shell completion or flags read
// from the environment, use github.com/urfave/cli rather than grow this.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/andycondon/loremason/internal/suggest"
)

// App is a program and its commands.
type App struct {
	Name     string // the program's name, such as mason
	Version  string // printed by --version
	Summary  string // what the program does, shown at the top of its help
	Footer   string // shown after the list of commands, such as notes on configuration
	Commands []*Command
}

// Command is one of an app's commands.
type Command struct {
	Name    string
	Summary string   // one line, shown in the app's help and the command's
	Args    []string // the arguments it takes, by name, such as "record ID"
	// Variadic makes the last of Args take one or more values, such as
	// "words", instead of exactly one. Without it the command takes exactly
	// the arguments in Args. The action gets all the values in args.
	Variadic bool
	// Setup declares the command's flags and returns what it does. The
	// action reads the flags' values through the variables Setup made.
	Setup func(fs *flag.FlagSet) Action
}

// Action runs a command. args are the arguments that aren't flags, already
// checked against Command.Args.
type Action func(env *Env, args []string) error

// Env is where a command writes.
type Env struct {
	Stdout, Stderr io.Writer
	app            string
}

// Warnf writes a warning to Stderr, after the program's name.
func (e *Env) Warnf(format string, args ...any) {
	fmt.Fprintf(e.Stderr, "%s: warning: %s\n", e.app, fmt.Sprintf(format, args...))
}

// ErrFailed makes Run exit with code 1 without printing anything more, for a
// command that has already said what went wrong.
var ErrFailed = errors.New("failed")

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Usagef returns an error for a mistake in how the command was called. Run
// prints it with a pointer to the command's help, and exits with code 2.
func Usagef(format string, args ...any) error {
	return &usageError{fmt.Sprintf(format, args...)}
}

// Run runs the app with args (without the program's name) and returns the
// exit code: 0 for success, 1 when the command failed, 2 for a mistake in
// the arguments. Help asked for is written to stdout; mistakes and errors to
// stderr.
func (a *App) Run(args []string, stdout, stderr io.Writer) int {
	env := &Env{Stdout: stdout, Stderr: stderr, app: a.Name}
	fs := quietFlags(a.Name)
	version := fs.Bool("version", false, "print the version and exit")
	switch err := fs.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		a.help(stdout, fs)
		return 0
	case err != nil:
		return a.exit(env, "", &usageError{err.Error()})
	case *version:
		fmt.Fprintf(stdout, "%s %s\n", a.Name, a.Version)
		return 0
	case fs.NArg() == 0:
		a.help(stdout, fs)
		return 0
	}
	name := fs.Arg(0)
	c := a.command(name)
	if c == nil {
		return a.exit(env, "", &usageError{fmt.Sprintf("unknown command `%s`%s", name, suggest.Hint(suggest.Near(name, a.names())))})
	}
	return a.exit(env, c.Name, c.run(a, env, fs.Args()[1:]))
}

func (c *Command) run(a *App, env *Env, args []string) error {
	fs := quietFlags(a.Name + " " + c.Name)
	action := c.Setup(fs)
	rest, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		c.help(a, env.Stdout, fs)
		return nil
	}
	if err != nil {
		return &usageError{err.Error()}
	}
	if len(rest) < len(c.Args) || (len(rest) > len(c.Args) && !c.Variadic) {
		return c.wrongCount(fs, len(rest))
	}
	return action(env, rest)
}

// parseInterspersed parses flags that may come before, between or after the
// other arguments, and returns the others. Everything after -- is an
// argument.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		left := fs.Args()
		if len(left) == 0 {
			return rest, nil
		}
		if used := len(args) - len(left); used > 0 && args[used-1] == "--" {
			return append(rest, left...), nil
		}
		rest, args = append(rest, left[0]), left[1:]
	}
}

func (c *Command) wrongCount(fs *flag.FlagSet, got int) error {
	var msg string
	switch {
	case len(c.Args) == 0:
		msg = c.Name + " takes no arguments"
	case c.Variadic && len(c.Args) == 1:
		msg = c.Name + " takes one or more " + c.Args[0]
	case c.Variadic:
		msg = fmt.Sprintf("%s takes %d arguments, the last of them one or more: %s", c.Name, len(c.Args), strings.Join(c.Args, ", "))
	case len(c.Args) == 1:
		msg = c.Name + " takes one " + c.Args[0]
	default:
		msg = fmt.Sprintf("%s takes %d arguments: %s", c.Name, len(c.Args), strings.Join(c.Args, ", "))
	}
	if got > len(c.Args) && !c.Variadic && takesValues(fs) {
		msg += " (quote a value that has spaces)"
	}
	return &usageError{msg}
}

// exit prints what err says, and returns the exit code for it. command is
// the command that ran, or "" for the app itself.
func (a *App) exit(env *Env, command string, err error) int {
	var usage *usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &usage):
		see := a.Name
		if command != "" {
			see += " " + command
		}
		fmt.Fprintf(env.Stderr, "%s: %s; see `%s --help`\n", a.Name, usage.msg, see)
		return 2
	case errors.Is(err, ErrFailed):
		return 1
	default:
		fmt.Fprintf(env.Stderr, "%s: %v\n", a.Name, err)
		return 1
	}
}

func (a *App) command(name string) *Command {
	for _, c := range a.Commands {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (a *App) names() []string {
	var names []string
	for _, c := range a.Commands {
		names = append(names, c.Name)
	}
	return names
}

// quietFlags makes a flag set that reports errors only by returning them, so
// Run can word them.
func quietFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

func takesValues(fs *flag.FlagSet) bool {
	values := false
	fs.VisitAll(func(f *flag.Flag) {
		if name, _ := flag.UnquoteUsage(f); name != "" {
			values = true
		}
	})
	return values
}
