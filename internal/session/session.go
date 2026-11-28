// Package session knows which session a command runs in, and attaches what
// it writes to the session's activity.
package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
)

// The variables that name the session.
const (
	EnvSession    = "LOREMASON_SESSION"
	EnvInitiative = "LOREMASON_INITIATIVE"
	EnvActor      = "LOREMASON_ACTOR"
)

// ErrNoInitiative is returned by Ensure when the session's activity doesn't
// exist yet and no initiative says where to put it.
var ErrNoInitiative = errors.New(EnvInitiative + " must name the initiative, since this session has no activity yet")

// Session is the session a command runs in, as the environment names it.
type Session struct {
	Key        string // whatever the harness calls the session
	Initiative string // slug, ID or alias; "" when unset
	Actor      string // who the activity is associated with; "" when unset
}

// FromEnv returns the session the environment names, or nil when
// LOREMASON_SESSION is unset or empty.
func FromEnv() (*Session, error) {
	key := os.Getenv(EnvSession)
	if key == "" {
		return nil, nil
	}
	return &Session{Key: key, Initiative: os.Getenv(EnvInitiative), Actor: os.Getenv(EnvActor)}, nil
}

// ActivityID is the ID of the session's activity. It comes from the key
// alone, so the file's path is known without loading the graph.
func (s *Session) ActivityID() string { return id.Derive("actv", s.Key) }

// Ensure makes sure the session's activity exists, and returns its path
// relative to root. Commands that start together all get the one file: the
// file is linked into place, which fails for every command but the first, and
// the others use what is there. A file for another session under the same ID
// is an error.
func (s *Session) Ensure(root string, reg *registry.Registry, now time.Time) (string, error) {
	typ, err := reg.Find("activity")
	if err != nil {
		return "", err
	}
	rid := s.ActivityID()
	rel := graph.RecordPath(typ, rid)
	p := filepath.Join(root, filepath.FromSlash(rel))

	if err := s.check(p, rid); err == nil {
		return rel, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if s.Initiative == "" {
		return "", ErrNoInitiative
	}
	x, err := index.Open(root, reg)
	if err != nil {
		return "", err
	}
	ini, err := x.ResolveInitiative(s.Initiative)
	x.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %w", EnvInitiative, err)
	}
	if ini.Slug == "" {
		ini.Slug = ini.ID
	}

	err = publish(p, s.render(rid, ini, now))
	if errors.Is(err, fs.ErrExist) {
		return rel, s.check(p, rid) // another command got there first
	}
	return rel, err
}

// check reads the activity file at p and reports whether it is this
// session's. A missing file gives an error that wraps fs.ErrNotExist.
func (s *Session) check(p, rid string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	rec, errs := record.Parse(data)
	if len(errs) > 0 {
		return fmt.Errorf("%s: %v", p, errs[0])
	}
	if got := rec.Header.Text("session"); got != s.Key {
		return fmt.Errorf("%s is the activity of session %q, not %q; two session keys share the ID %s", p, got, s.Key, rid)
	}
	return nil
}

func (s *Session) render(rid string, ini index.Initiative, now time.Time) string {
	day := now.Format(time.DateOnly)
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\nsession: %s\nstarted_at: %s\npart_of: %s\n",
		rid, record.Quote(fmt.Sprintf("Session %s (%s)", day, ini.Slug)), record.Quote(s.Key), now.UTC().Format(time.RFC3339), ini.ID)
	if s.Actor != "" {
		fmt.Fprintf(&b, "associated_with: [%s]\n", s.Actor)
	}
	fmt.Fprintf(&b, "created: %s\n---\n", day)
	return b.String()
}

// publish writes text to a new file at p, failing with fs.ErrExist when the
// file is there. It writes a temporary file and links it, so no other command
// ever reads a half-written file.
func publish(p, text string) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".new-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(text)
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Link(f.Name(), p)
}
