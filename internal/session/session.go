// Package session knows which session a command runs in, and attaches what
// it writes to the session's activity.
package session

import (
	"errors"
	"fmt"
	"io"
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

	// Warn is where Ensure writes a note about what it ignored. Nil writes none.
	Warn io.Writer
}

// FromEnv returns the session the environment names, or nil when
// LOREMASON_SESSION is unset or blank. A key that spans lines, or an actor
// that isn't a reference, is an error, which callers treat as a usage error:
// either would be written into the activity's header and break the file.
func FromEnv() (*Session, error) {
	key := strings.TrimSpace(os.Getenv(EnvSession))
	if key == "" {
		return nil, nil
	}
	if strings.ContainsAny(key, "\r\n") {
		return nil, fmt.Errorf("%s must be one line", EnvSession)
	}
	actor := os.Getenv(EnvActor)
	if actor != "" {
		if _, err := graph.ParseRef(actor); err != nil {
			return nil, fmt.Errorf("%s: %w", EnvActor, err)
		}
	}
	return &Session{Key: key, Initiative: os.Getenv(EnvInitiative), Actor: actor}, nil
}

// ActivityID is the ID of the session's activity. It comes from the key
// alone, so the file's path is known without loading the graph.
func (s *Session) ActivityID() string { return id.Derive("actv", s.Key) }

// Ensure makes sure the session's activity exists, and returns its path
// relative to root. Commands that start together all get one file: each tries
// to link its own copy into place, only the first succeeds, and the others
// use what is there. A file for another session under the same ID is an
// error.
func (s *Session) Ensure(root string, reg *registry.Registry, now time.Time) (string, error) {
	typ, err := reg.Find("activity")
	if err != nil {
		return "", err
	}
	rid := s.ActivityID()
	rel := graph.RecordPath(typ, rid)
	p := filepath.Join(root, filepath.FromSlash(rel))

	partOf, err := s.check(p, rid)
	if err == nil {
		return rel, s.warnIfElsewhere(root, reg, partOf)
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
		_, err = s.check(p, rid) // another command got there first
		return rel, err
	}
	return rel, err
}

// warnIfElsewhere writes a note when LOREMASON_INITIATIVE names a different
// initiative from the activity's. Once the activity exists its initiative
// wins, so the variable is ignored. It opens the index only when the variable
// is set.
func (s *Session) warnIfElsewhere(root string, reg *registry.Registry, partOf string) error {
	if s.Initiative == "" || s.Warn == nil {
		return nil
	}
	x, err := index.Open(root, reg)
	if err != nil {
		return err
	}
	defer x.Close()
	asked, err := x.ResolveInitiative(s.Initiative)
	if err != nil {
		return fmt.Errorf("%s: %w", EnvInitiative, err)
	}
	if asked.ID == partOf {
		return nil
	}
	has := partOf
	if cur, err := x.ResolveInitiative(partOf); err == nil && cur.Slug != "" {
		has = cur.Slug
	}
	fmt.Fprintf(s.Warn, "note: this session's activity belongs to %s already, so %s=%s is ignored\n", has, EnvInitiative, s.Initiative)
	return nil
}

// check reads the activity file at p and reports whether it is this
// session's, returning its part_of. A missing file gives an error that wraps
// fs.ErrNotExist.
func (s *Session) check(p, rid string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	rec, errs := record.Parse(data)
	if len(errs) > 0 {
		return "", fmt.Errorf("%s: %v", p, errs[0])
	}
	if got := rec.Header.Text("session"); got != s.Key {
		return "", fmt.Errorf("%s is the activity of session %q, not %q; two session keys share the ID %s", p, got, s.Key, rid)
	}
	return rec.Header.Text("part_of"), nil
}

// render writes the activity. Its header fields must agree with the activity
// template in schema/types.yaml, which `mason new activity` uses.
func (s *Session) render(rid string, ini index.Initiative, now time.Time) string {
	day := now.Format(time.DateOnly)
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\nsession: %s\nstarted_at: %s\npart_of: %s\n",
		rid, record.Quote(fmt.Sprintf("Session %s (%s)", day, ini.Slug)), record.Quote(s.Key), now.Format(time.RFC3339), ini.ID)
	if s.Actor != "" {
		// record.Quote only knows block context, so write a block list; in
		// a flow list a reference such as `a],b` would break the line.
		b.WriteString(record.Render("associated_with", record.List(record.Str(s.Actor))))
	}
	fmt.Fprintf(&b, "created: %s\n---\n", day)
	return b.String()
}

// publish writes text to a new file at p, failing with fs.ErrExist when the
// file is already there. It writes a temporary file and then links it, so no
// other command reads a half-written file. It needs a file system with hard
// links: a link fails when the name is taken, which is how commands that
// start together agree on one file.
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
	// CreateTemp makes the file 0600; records are 0644, as graph.Create writes them.
	if err := os.Chmod(f.Name(), 0o644); err != nil {
		return err
	}
	return os.Link(f.Name(), p)
}
