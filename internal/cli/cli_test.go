package cli

import (
	"bytes"
	"errors"
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// greet is a small app with a command of each shape: no arguments, one
// argument with a flag that takes a value, and one that fails.
func greet(got *[]string) *App {
	return &App{
		Name:    "greet",
		Version: "1.2.3",
		Summary: "greet says hello.",
		Footer:  "Set GREETING to change the word.",
		Commands: []*Command{
			{
				Name:    "hello",
				Summary: "say hello to someone",
				Args:    []string{"name"},
				Setup: func(fs *flag.FlagSet) Action {
					times := fs.Int("times", 1, "say it `n` times")
					loud := fs.Bool("loud", false, "shout")
					return func(env *Env, args []string) error {
						*got = append(*got, args[0], strconv.Itoa(*times), strconv.FormatBool(*loud))
						return nil
					}
				},
			},
			{
				Name:    "wave",
				Summary: "wave",
				Setup:   func(*flag.FlagSet) Action { return func(*Env, []string) error { return nil } },
			},
			{
				Name:    "fail",
				Summary: "fail in each way",
				Args:    []string{"how"},
				Setup: func(*flag.FlagSet) Action {
					return func(env *Env, args []string) error {
						switch args[0] {
						case "usage":
							return Usagef("that's not how")
						case "quiet":
							env.Warnf("about to fail")
							return ErrFailed
						}
						return errors.New("it broke")
					}
				},
			},
		},
	}
}

func run(args ...string) (code int, got []string, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = greet(&got).Run(args, &out, &errb)
	return code, got, out.String(), errb.String()
}

func TestFlagsAnywhere(t *testing.T) {
	for _, args := range [][]string{
		{"hello", "--times", "3", "--loud", "ann"},
		{"hello", "ann", "--times", "3", "--loud"},
		{"hello", "--loud", "ann", "--times=3"},
	} {
		if code, got, _, errOut := run(args...); code != 0 || !slices.Equal(got, []string{"ann", "3", "true"}) {
			t.Errorf("%v: exit %d, got %v, %s", args, code, got, errOut)
		}
	}
}

func TestEverythingAfterDoubleDashIsAnArgument(t *testing.T) {
	if code, got, _, errOut := run("hello", "--", "--loud"); code != 0 || !slices.Equal(got, []string{"--loud", "1", "false"}) {
		t.Errorf("exit %d, got %v, %s", code, got, errOut)
	}
}

func TestMistakes(t *testing.T) {
	for want, args := range map[string][]string{
		"greet: hello takes one name; see `greet hello --help`":                                 {"hello"},
		"greet: hello takes one name (quote a value that has spaces); see `greet hello --help`": {"hello", "ann", "lee"},
		"greet: wave takes no arguments; see `greet wave --help`":                               {"wave", "now"},
		"greet: flag provided but not defined: -nope; see `greet hello --help`":                 {"hello", "ann", "--nope"},
		"greet: flag provided but not defined: -nope; see `greet --help`":                       {"--nope"},
		"greet: unknown command `helo`; did you mean `hello`?; see `greet --help`":              {"helo"},
		"greet: that's not how; see `greet fail --help`":                                        {"fail", "usage"},
	} {
		code, got, out, errOut := run(args...)
		if code != 2 || errOut != want+"\n" || out != "" || got != nil {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
}

func TestFailures(t *testing.T) {
	if code, _, _, errOut := run("fail", "error"); code != 1 || errOut != "greet: it broke\n" {
		t.Errorf("an error: exit %d, %q", code, errOut)
	}
	if code, _, _, errOut := run("fail", "quiet"); code != 1 || errOut != "greet: warning: about to fail\n" {
		t.Errorf("ErrFailed: exit %d, %q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	if code, _, out, _ := run("--version"); code != 0 || out != "greet 1.2.3\n" {
		t.Errorf("exit %d, %q", code, out)
	}
}

const appHelp = "greet says hello.\n\nUsage:\n  greet <command> [arguments]\n\nCommands:\n" +
	"  hello   say hello to someone\n" +
	"  wave    wave\n" +
	"  fail    fail in each way\n" +
	"\nRun `greet <command> --help` for a command's arguments and flags.\n" +
	"\nSet GREETING to change the word.\n" +
	"\nFlags:\n" +
	"  --version   print the version and exit\n" +
	"  --help      show this help\n"

const helloHelp = "Usage:\n  greet hello <name> [flags]\n\nsay hello to someone\n" +
	"\nFlags:\n" +
	"  --loud        shout\n" +
	"  --times <n>   say it n times\n" +
	"  --help        show this help\n"

func TestHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, appHelp},
		{nil, appHelp},
		{[]string{"hello", "--help"}, helloHelp},
		{[]string{"hello", "ann", "-h"}, helloHelp},
	} {
		if code, got, out, errOut := run(tc.args...); code != 0 || out != tc.want || errOut != "" || got != nil {
			t.Errorf("%v: exit %d, stderr %q, stdout:\n%s", tc.args, code, errOut, out)
		}
	}
}

// The package stays general: only the standard library and package suggest.
func TestImportsNothingFromLoremason(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(strings.Split(path, "/")[0], ".") && path != "github.com/andycondon/loremason/internal/suggest" {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}

// echo has a command that takes one or more words, and one that takes a
// name and then one or more words.
func echo(got *[]string) *App {
	setup := func(*flag.FlagSet) Action {
		return func(_ *Env, args []string) error { *got = append(*got, args...); return nil }
	}
	return &App{
		Name:    "echo",
		Version: "1",
		Summary: "echo repeats.",
		Commands: []*Command{
			{Name: "say", Summary: "say words", Args: []string{"words"}, Variadic: true, Setup: setup},
			{Name: "tell", Summary: "tell someone words", Args: []string{"name", "words"}, Variadic: true, Setup: setup},
		},
	}
}

func runEcho(args ...string) (code int, got []string, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = echo(&got).Run(args, &out, &errb)
	return code, got, out.String(), errb.String()
}

func TestVariadicTakesOneOrMore(t *testing.T) {
	if code, got, _, _ := runEcho("say", "a"); code != 0 || !slices.Equal(got, []string{"a"}) {
		t.Errorf("one word: exit %d, got %v", code, got)
	}
	if code, got, _, _ := runEcho("say", "a", "b", "c"); code != 0 || !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("exit %d, got %v", code, got)
	}
	if code, got, _, _ := runEcho("tell", "ann", "x", "y"); code != 0 || !slices.Equal(got, []string{"ann", "x", "y"}) {
		t.Errorf("exit %d, got %v", code, got)
	}
}

func TestVariadicNeedsAtLeastOne(t *testing.T) {
	for want, args := range map[string][]string{
		"echo: say takes one or more words; see `echo say --help`":                                        {"say"},
		"echo: tell takes 2 arguments, the last of them one or more: name, words; see `echo tell --help`": {"tell", "ann"},
	} {
		if code, got, out, errOut := runEcho(args...); code != 2 || errOut != want+"\n" || out != "" || got != nil {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
}

func TestVariadicHelpShowsTheRepeat(t *testing.T) {
	_, _, out, _ := runEcho("say", "--help")
	if !strings.Contains(out, "echo say <words>...") {
		t.Errorf("help:\n%s", out)
	}
}
