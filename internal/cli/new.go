package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

func runNew(args []string, stdout, stderr io.Writer) int {
	flags := newFlags(stderr)
	title := flags.String("title", "", "the record's title")
	// Flags may come before or after the type.
	var names []string
	for rest := args; ; {
		if code, done := parse(flags, rest); done {
			return code
		}
		if flags.NArg() == 0 {
			break
		}
		names, rest = append(names, flags.Arg(0)), flags.Args()[1:]
	}
	if len(names) != 1 {
		return misuse(stderr, "new takes one record type (quote a title that has spaces)")
	}
	reg := registry.Load()
	typ := reg.Types[strings.ReplaceAll(names[0], "-", "_")]
	if typ == nil {
		hint := "the types are " + strings.Join(reg.Names(), ", ")
		if s := suggest.Near(names[0], reg.Names()); s != "" {
			hint = fmt.Sprintf("did you mean `%s`?", s)
		}
		fmt.Fprintf(stderr, "mason: unknown record type `%s`; %s\n", names[0], hint)
		return 2
	}
	if strings.TrimSpace(*title) == "" || strings.ContainsFunc(*title, unicode.IsControl) {
		return misuse(stderr, "new needs --title with one line of text")
	}
	w, err := open(stderr)
	if err != nil {
		return fail(stderr, err)
	}
	dir := filepath.Join(w.Graph(), typ.Folder())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(stderr, err)
	}
	for {
		rid := id.New(typ.Prefix)
		path := filepath.Join(dir, rid+".md")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fail(stderr, err)
		}
		_, err = f.WriteString(render(typ, rid, strings.TrimSpace(*title), time.Now().Format(time.DateOnly)))
		if err := errors.Join(err, f.Close()); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, path)
		return 0
	}
}

// render writes a new record.
func render(typ *registry.Type, rid, title, today string) string {
	yamlTitle := quote(title)
	fill := strings.NewReplacer("{title}", yamlTitle, "{slug}", quote(slug(title)), "{today}", today)
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\n%screated: %s\n---\n", rid, yamlTitle, fill.Replace(typ.Template), today)
	if typ.Body != "" {
		b.WriteString("\n" + typ.Body)
	}
	return b.String()
}

// slug turns a title into a kebab-case name.
func slug(title string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	if len(words) == 0 {
		return "untitled"
	}
	return strings.Join(words, "-")
}

// quote writes text as a YAML value, with quotes when YAML needs them.
func quote(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		panic(err) // a string always marshals
	}
	return strings.TrimSuffix(string(out), "\n")
}
