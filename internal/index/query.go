package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/andycondon/loremason/internal/suggest"
)

// Summary is a record in one line: enough to name it in a list.
type Summary struct {
	ID, Type, Title, Status string
}

// Pin is the revision of a record that a link was written against.
type Pin struct {
	Rev int
	FP  string // the fingerprint, or "" when the link pinned none
}

// Link is one end of a link, seen from the record that was asked about.
type Link struct {
	Field string  // the field the link is written in, or "subrecord_of" for containment
	Other Summary // the record at the other end
	Pin   *Pin    // nil when the link isn't pinned
}

// Detail is everything stored about one record.
type Detail struct {
	Summary
	Header string // the record's header as JSON, "" when its file couldn't be read
	Body   string
	Out    []Link // links the record makes, by field then target
	In     []Link // links to the record, by field then source
}

// Dependent is a record that relies on another, directly or through others.
type Dependent struct {
	Summary
	Depth int     // 1 for a record that links to it, 2 for one that links to that, and so on
	Field string  // the field of the link that reached it
	On    Summary // the record that link points at; the one asked about when Depth is 1
}

// StaleGroup is a record that has changed and the links still pinned to
// what it was.
type StaleGroup struct {
	Target   Summary
	Revision int    // the target's current revision
	Links    []Link // Other is the record holding the link
}

// Reopening is a commitment resting on a belief that has fallen.
type Reopening struct {
	Commitment Summary
	Belief     Summary
	Field      string // the field of the commitment that links to the belief
	Status     string // the belief's status
}

// ErrNotFound is what a NotFoundError unwraps to.
var ErrNotFound = errors.New("no such record")

// NotFoundError says no record has the ID, and which names are close.
type NotFoundError struct {
	ID    string
	Close []string // up to three IDs or aliases near ID, nearest first
}

func (e *NotFoundError) Error() string {
	near := ""
	if len(e.Close) > 0 {
		near = suggest.Hint(strings.Join(e.Close, "`, `"))
	}
	return fmt.Sprintf("no record is called `%s`%s", e.ID, near)
}

// Unwrap returns ErrNotFound.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

const summaryCols = `n.id, n.type, n.title, n.status`

func scanSummary(r interface{ Scan(...any) error }, extra ...any) (Summary, error) {
	var s Summary
	err := r.Scan(append([]any{&s.ID, &s.Type, &s.Title, &s.Status}, extra...)...)
	return s, err
}

// resolve returns the ID that name stands for: an ID, an alias, or an alias
// with a local name after a slash, as elsewhere in the graph.
func (x *Index) resolve(name string) (string, error) {
	for _, q := range []string{
		`SELECT id FROM nodes WHERE id = ?`,
		`SELECT id FROM aliases WHERE alias = ?`,
	} {
		var id string
		err := x.db.QueryRow(q, name).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		head, local := name[:i], name[i+1:]
		var id string
		err := x.db.QueryRow(`SELECT id FROM nodes WHERE id = (SELECT id FROM aliases WHERE alias = ?) || '/' || ?`, head, local).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return "", x.notFound(name)
}

func (x *Index) notFound(name string) error {
	rows, err := x.db.Query(`SELECT id FROM nodes UNION SELECT alias FROM aliases ORDER BY 1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		names = append(names, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	e := &NotFoundError{ID: name}
	for len(e.Close) < 3 {
		c := suggest.Near(name, names)
		if c == "" {
			break
		}
		e.Close = append(e.Close, c)
		for i, n := range names {
			if n == c {
				names = append(names[:i], names[i+1:]...)
				break
			}
		}
	}
	return e
}

// Find returns the records whose title or body has all of the words, by ID.
// The words are plain text: punctuation separates them, so "utf-8 files"
// finds a body that says "non-UTF-8 files".
func (x *Index) Find(words string) ([]Summary, error) {
	var terms []string
	for _, w := range strings.FieldsFunc(words, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		terms = append(terms, `"`+w+`"`)
	}
	if len(terms) == 0 {
		return nil, nil
	}
	rows, err := x.db.Query(`SELECT `+summaryCols+` FROM nodes n
		WHERE n.id IN (SELECT id FROM nodes_fts WHERE nodes_fts MATCH ?) ORDER BY n.id`, strings.Join(terms, " "))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Show returns everything stored about the record called id, which may be an
// alias. An unknown name gives a *NotFoundError.
func (x *Index) Show(id string) (*Detail, error) {
	id, err := x.resolve(id)
	if err != nil {
		return nil, err
	}
	d := &Detail{}
	err = x.db.QueryRow(`SELECT `+summaryCols+`, n.header, n.body FROM nodes n WHERE n.id = ?`, id).Scan(
		&d.ID, &d.Type, &d.Title, &d.Status, &d.Header, &d.Body)
	if err != nil {
		return nil, err
	}
	if d.Out, err = x.links(`e.src = ?`, `e.dst`, id); err != nil {
		return nil, err
	}
	if d.In, err = x.links(`e.dst = ?`, `e.src`, id); err != nil {
		return nil, err
	}
	return d, nil
}

// links returns the links where, selecting by where, the record at the end
// named by other is the one to report.
func (x *Index) links(where, other, id string) ([]Link, error) {
	rows, err := x.db.Query(`SELECT e.name, e.pinned_rev, e.pinned_fp, `+summaryCols+`
		FROM edges e JOIN nodes n ON n.id = `+other+` WHERE `+where+` ORDER BY e.name, n.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

// scanLinks reads rows of name, pinned_rev, pinned_fp and a summary.
func scanLinks(rows *sql.Rows) ([]Link, error) {
	var out []Link
	for rows.Next() {
		var l Link
		var rev sql.NullInt64
		var fp string
		if err := rows.Scan(&l.Field, &rev, &fp, &l.Other.ID, &l.Other.Type, &l.Other.Title, &l.Other.Status); err != nil {
			return nil, err
		}
		if rev.Valid {
			l.Pin = &Pin{Rev: int(rev.Int64), FP: fp}
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Dependents returns the records that rely on the one called id, which may
// be an alias: those with a propagating link to it, those with one to them,
// and so on. Each is listed once, at its shortest depth, by depth then ID.
// A link that only mentions the record doesn't count, nor does containment.
func (x *Index) Dependents(id string) ([]Dependent, error) {
	id, err := x.resolve(id)
	if err != nil {
		return nil, err
	}
	rows, err := x.db.Query(`
WITH RECURSIVE dep(id, depth, via, field) AS (
	SELECT src, 1, dst, name FROM edges WHERE dst = ?1 AND propagates = 1
	UNION
	SELECT e.src, d.depth + 1, e.dst, e.name FROM edges e JOIN dep d ON e.dst = d.id
	WHERE e.propagates = 1 AND d.depth < (SELECT count(*) FROM nodes)
), first AS (
	SELECT id, depth, via, field, ROW_NUMBER() OVER (PARTITION BY id ORDER BY depth, via, field) AS rn FROM dep WHERE id != ?1
)
SELECT n.id, n.type, n.title, n.status, f.depth, f.field, v.id, v.type, v.title, v.status
FROM first f JOIN nodes n ON n.id = f.id JOIN nodes v ON v.id = f.via
WHERE f.rn = 1 ORDER BY f.depth, n.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dependent
	for rows.Next() {
		var d Dependent
		if err := rows.Scan(&d.ID, &d.Type, &d.Title, &d.Status, &d.Depth, &d.Field,
			&d.On.ID, &d.On.Type, &d.On.Title, &d.On.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Stale returns the records that have changed since links were written
// against them, one group each by ID, with the links that are out of date.
func (x *Index) Stale() ([]StaleGroup, error) {
	rows, err := x.db.Query(`SELECT t.id, t.type, t.title, t.status, COALESCE(t.revision, 0),
		e.name, e.pinned_rev, e.pinned_fp, s.id, s.type, s.title, s.status
		FROM edges e JOIN nodes t ON t.id = e.dst JOIN nodes s ON s.id = e.src
		WHERE e.stale = 1 ORDER BY t.id, s.id, e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaleGroup
	for rows.Next() {
		var t Summary
		var rev int
		var l Link
		var pinRev sql.NullInt64
		var fp string
		if err := rows.Scan(&t.ID, &t.Type, &t.Title, &t.Status, &rev,
			&l.Field, &pinRev, &fp, &l.Other.ID, &l.Other.Type, &l.Other.Title, &l.Other.Status); err != nil {
			return nil, err
		}
		if pinRev.Valid {
			l.Pin = &Pin{Rev: int(pinRev.Int64), FP: fp}
		}
		if n := len(out); n == 0 || out[n-1].Target.ID != t.ID {
			out = append(out, StaleGroup{Target: t, Revision: rev})
		}
		g := &out[len(out)-1]
		g.Links = append(g.Links, l)
	}
	return out, rows.Err()
}

// Reopened returns the commitments resting on a belief that has fallen, by
// commitment then belief.
func (x *Index) Reopened() ([]Reopening, error) {
	rows, err := x.db.Query(`SELECT c.id, c.type, c.title, c.status, e.name, b.id, b.type, b.title, b.status
		FROM edges e JOIN nodes c ON c.id = e.src JOIN nodes b ON b.id = e.dst
		WHERE e.reopens = 1 ORDER BY c.id, b.id, e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reopening
	for rows.Next() {
		var r Reopening
		if err := rows.Scan(&r.Commitment.ID, &r.Commitment.Type, &r.Commitment.Title, &r.Commitment.Status, &r.Field,
			&r.Belief.ID, &r.Belief.Type, &r.Belief.Title, &r.Belief.Status); err != nil {
			return nil, err
		}
		r.Status = r.Belief.Status
		out = append(out, r)
	}
	return out, rows.Err()
}
