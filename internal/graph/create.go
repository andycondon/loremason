package graph

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
)

// ErrTitle is returned by Create when a title isn't one line of text.
var ErrTitle = errors.New("a title must be one line of text")

// Create writes a new record of type t with a fresh ID, filled in from the
// type's template, and returns its path relative to root. Spaces around the
// title are dropped.
func Create(root string, t *registry.Type, title string, today time.Time) (string, error) {
	title, err := cleanTitle(title)
	if err != nil {
		return "", err
	}
	for {
		rid := id.New(t.Prefix)
		rel := RecordPath(t, rid)
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue // another record has this ID; draw again
		}
		if err != nil {
			return "", err
		}
		_, err = f.WriteString(render(t, rid, title, today.Format(time.DateOnly)))
		if err := errors.Join(err, f.Close()); err != nil {
			return "", err
		}
		return rel, nil
	}
}

// cleanTitle applies the schemas' rule for a title, one line of text, and
// trims it.
func cleanTitle(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w, and this one isn't valid UTF-8", ErrTitle)
	}
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsFunc(s, unicode.IsControl) {
		return "", ErrTitle
	}
	return s, nil
}

// render writes a new record: id, title, the type's template, created, and
// the type's body.
func render(t *registry.Type, rid, title, today string) string {
	fill := strings.NewReplacer("{title}", record.Quote(title), "{slug}", record.Quote(slug(title, rid)), "{today}", today)
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\n%screated: %s\n---\n", rid, record.Quote(title), fill.Replace(t.Template), today)
	if t.Body != "" {
		b.WriteString("\n" + t.Body)
	}
	return b.String()
}

// slug turns a title into a kebab-case name. A title with no letters or
// digits it can use gets one made from the ID, so it can't clash.
func slug(title, rid string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	if len(words) == 0 {
		return strings.ReplaceAll(rid, "_", "-")
	}
	return strings.Join(words, "-")
}
