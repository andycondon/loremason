package index

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
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
	Field string  // the field the link is written in
	Local string  // for a link made by one of the record's sub-records, that sub-record's local name ("s1"); otherwise ""
	Other Summary // the record at the other end
	Pin   *Pin    // nil when the link isn't pinned
}

// Detail is everything stored about one record.
type Detail struct {
	Summary
	Header string // the record's header as JSON; "" for a sub-record, or when its file couldn't be read
	Body   string // "" for a sub-record, or when its file couldn't be read
	Out    []Link // links the record and its sub-records make, by field then target; a sub-record's carry its Local name
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
	Revision int         // the target's current revision
	Links    []StaleLink // Other is the record holding the link
}

// StaleLink is a link to a changed record that needs a re-check.
type StaleLink struct {
	Link
	Stale    bool // the link is pinned to a revision the target has moved past
	Orphaned bool // the link rests on a target that is no longer in force
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
	return x.notFoundIn(name, `SELECT id FROM nodes UNION SELECT alias FROM aliases ORDER BY 1`)
}

// notFoundIn is the error for a name that matches none of the names the
// query selects, with the nearest of them.
func (x *Index) notFoundIn(name, query string) error {
	rows, err := x.db.Query(query)
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

// Words returns the words Find searches for in text: runs of letters and
// digits. It is empty when the text has none, as when it is only punctuation.
func Words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Find returns the records whose title or body has all of the words, by ID.
// The words are plain text: punctuation separates them, so "utf-8 files"
// finds a body that says "non-UTF-8 files".
func (x *Index) Find(words string) ([]Summary, error) {
	var terms []string
	for _, w := range Words(words) {
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
	if d.Out, err = x.out(id); err != nil {
		return nil, err
	}
	if d.In, err = x.in(id); err != nil {
		return nil, err
	}
	return d, nil
}

// out returns the links made by the record and by its sub-records. Each of
// a sub-record's has Local set. Containment itself isn't a link to report.
func (x *Index) out(id string) ([]Link, error) {
	rows, err := x.db.Query(`SELECT e.name, e.pinned_rev, e.pinned_fp, `+summaryCols+`, e.src
		FROM edges e JOIN nodes n ON n.id = e.dst
		WHERE e.name != ?2 AND (e.src = ?1 OR e.src IN (SELECT c.src FROM edges c WHERE c.dst = ?1 AND c.name = ?2))
		ORDER BY e.name, n.id, e.src`, id, Containment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		var rev sql.NullInt64
		var fp, src string
		if err := rows.Scan(&l.Field, &rev, &fp, &l.Other.ID, &l.Other.Type, &l.Other.Title, &l.Other.Status, &src); err != nil {
			return nil, err
		}
		if rev.Valid {
			l.Pin = &Pin{Rev: int(rev.Int64), FP: fp}
		}
		if src != id {
			l.Local = strings.TrimPrefix(src, id+"/")
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// in returns the links to the record, without containment.
func (x *Index) in(id string) ([]Link, error) {
	rows, err := x.db.Query(`SELECT e.name, e.pinned_rev, e.pinned_fp, `+summaryCols+`
		FROM edges e JOIN nodes n ON n.id = e.src WHERE e.dst = ? AND e.name != ? ORDER BY e.name, n.id`, id, Containment)
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
// and so on. Each is listed once, at its shortest depth, by depth then ID,
// so a cycle or a dense tangle of links costs no more than its links. The
// link that reached a record is the one from the lowest-numbered record on
// the previous level, then by field. A link that only mentions the record
// doesn't count, nor does containment.
func (x *Index) Dependents(id string) ([]Dependent, error) {
	id, err := x.resolve(id)
	if err != nil {
		return nil, err
	}
	// The walk is traversal only; which links propagate is stored.
	rows, err := x.db.Query(`SELECT dst, src, name FROM edges WHERE propagates = 1 ORDER BY dst, src, name`)
	if err != nil {
		return nil, err
	}
	type reach struct{ src, field string }
	in := map[string][]reach{} // by the record linked to
	for rows.Next() {
		var dst string
		var r reach
		if err := rows.Scan(&dst, &r.src, &r.field); err != nil {
			rows.Close()
			return nil, err
		}
		in[dst] = append(in[dst], r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	type found struct {
		id, via, field string
		depth          int
	}
	seen := map[string]bool{id: true}
	var order []found
	frontier := []string{id} // sorted, so the first link found to a record has the lowest via
	for depth := 1; len(frontier) > 0; depth++ {
		var next []found
		for _, via := range frontier {
			for _, r := range in[via] {
				if !seen[r.src] {
					seen[r.src] = true
					next = append(next, found{r.src, via, r.field, depth})
				}
			}
		}
		sort.Slice(next, func(i, j int) bool { return next[i].id < next[j].id })
		order = append(order, next...)
		frontier = frontier[:0]
		for _, f := range next {
			frontier = append(frontier, f.id)
		}
	}
	sum, err := x.db.Prepare(`SELECT ` + summaryCols + ` FROM nodes n WHERE n.id = ?`)
	if err != nil {
		return nil, err
	}
	defer sum.Close()
	summary := func(id string) (Summary, error) { return scanSummary(sum.QueryRow(id)) }
	out := make([]Dependent, 0, len(order))
	for _, f := range order {
		d := Dependent{Depth: f.depth, Field: f.field}
		if d.Summary, err = summary(f.id); err != nil {
			return nil, err
		}
		if d.On, err = summary(f.via); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Stale returns the records that have changed since links were written
// against them, or that are no longer in force under a link that relies on
// them, one group per record by ID, with the links that are out of date.
func (x *Index) Stale() ([]StaleGroup, error) {
	rows, err := x.db.Query(`SELECT t.id, t.type, t.title, t.status, COALESCE(t.revision, 0),
		e.name, e.pinned_rev, e.pinned_fp, e.stale, e.orphaned, s.id, s.type, s.title, s.status
		FROM edges e JOIN nodes t ON t.id = e.dst JOIN nodes s ON s.id = e.src
		WHERE e.stale = 1 OR e.orphaned = 1 ORDER BY t.id, s.id, e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaleGroup
	for rows.Next() {
		var t Summary
		var rev int
		var l StaleLink
		var pinRev sql.NullInt64
		var fp string
		if err := rows.Scan(&t.ID, &t.Type, &t.Title, &t.Status, &rev,
			&l.Field, &pinRev, &fp, &l.Stale, &l.Orphaned, &l.Other.ID, &l.Other.Type, &l.Other.Title, &l.Other.Status); err != nil {
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
