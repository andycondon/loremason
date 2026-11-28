package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// View is where an initiative stands: the answer to mason where. The records
// in it are the initiative's members, which are the records whose part_of
// links lead to it. Every list is sorted by ID.
type View struct {
	Initiative Summary
	Slug       string
	InProgress []Summary  // members with progress in_progress
	Blocked    []Blocked  // members with progress blocked
	Open       []Question // member questions nothing settles
	Recheck    []Change   // records that changed under the members' links, by record
}

// Blocked is a task that can't go on, and what it says it waits on.
type Blocked struct {
	Task Summary
	By   []Summary // the targets of its blocked_by links; none when it names no blocker
}

// Question is a question still open, with the answers waiting for it.
type Question struct {
	Summary
	Pending []Summary // the sources of its pending answers links, such as a proposed or leaning decision
}

// Change is a record that has changed, and the members' links that need a
// re-check because of it.
type Change struct {
	Target   Summary
	Revision int           // the target's current revision
	Links    []RecheckLink // by the record holding the link, then field
}

// RecheckLink is a link from a member, or from one of its sub-records, that
// needs a re-check.
type RecheckLink struct {
	Link         // Other is the record holding the link
	Stale   bool // the link is pinned to a revision the target has moved past
	Reopens bool // the link reopens its source, because the target has fallen
	// Orphaned: the link rests on a target that is no longer in force, such as
	// a withdrawn belief or a superseded task.
	Orphaned bool
	// Doubted: the link relies on a belief whose support is undercut or whose
	// evidence is undermined.
	Doubted bool
}

// Overview is what an initiative's README shows.
type Overview struct {
	Initiative Summary
	Slug       string
	Path       string // the initiative's record file, relative to the workspace, as stored
	Purpose    string
	Body       string
	Goals      []string
	People     []Person // in the order the record lists them
	DesignDocs []Entry
	PRDs       []Entry
	Tasks      []Entry
	Findings   []Entry // findings produced in the initiative's activities; not members, and with no status
}

// Person is someone an initiative involves, and their role in it.
type Person struct {
	Summary
	Role string // "" when the link names none
}

// Entry is a member record listed on an initiative's page, by title (ignoring
// ASCII case) then ID.
type Entry struct {
	Summary
	Path     string // the record's file, relative to the workspace, as stored
	Progress string // a task's progress; "" for other records
}

// DuplicateSlugError says two initiatives have the same slug, so the slug
// doesn't name one. The IDs do.
type DuplicateSlugError struct {
	Slug string
	IDs  []string // sorted
}

func (e *DuplicateSlugError) Error() string {
	return fmt.Sprintf("more than one initiative is called `%s`: %s", e.Slug, strings.Join(e.IDs, ", "))
}

// NotInitiativeError says a record exists but isn't an initiative.
type NotInitiativeError struct {
	ID, Type string
}

func (e *NotInitiativeError) Error() string {
	article := "a"
	if strings.ContainsAny(e.Type[:1], "aeiou") {
		article = "an"
	}
	return fmt.Sprintf("%s is %s %s, not an initiative", e.ID, article, e.Type)
}

// slugsAndIDs selects the names a mistyped initiative is compared with.
const slugsAndIDs = `SELECT slug FROM initiatives WHERE slug != '' UNION SELECT id FROM initiatives ORDER BY 1`

// bySlug returns the ID of the initiative with the slug. It returns "" when
// there is none, or when the slug is empty (an initiative with no slug has
// none to be found by), and a *DuplicateSlugError when there are several.
func (x *Index) bySlug(slug string) (string, error) {
	if slug == "" {
		return "", nil
	}
	rows, err := x.db.Query(`SELECT id FROM initiatives WHERE slug = ? ORDER BY id`, slug)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		found = append(found, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	}
	return "", &DuplicateSlugError{Slug: slug, IDs: found}
}

// initiative returns the ID of the initiative that name stands for: its
// slug, its ID or an alias. A name that is none of these gives a
// *NotFoundError with the nearest slugs and IDs, and a record that isn't an
// initiative gives a *NotInitiativeError.
func (x *Index) initiative(name string) (string, error) {
	if id, err := x.bySlug(name); err != nil || id != "" {
		return id, err
	}
	id, err := x.resolve(name)
	if errors.Is(err, ErrNotFound) {
		return "", x.notFoundIn(name, slugsAndIDs)
	}
	if err != nil {
		return "", err
	}
	var typ string
	if err := x.db.QueryRow(`SELECT type FROM nodes WHERE id = ?`, id).Scan(&typ); err != nil {
		return "", err
	}
	if typ != "initiative" {
		return "", &NotInitiativeError{ID: id, Type: typ}
	}
	return id, nil
}

// summaries runs a query that selects summaryCols from nodes n, and returns
// the records in the order it gives them.
func (x *Index) summaries(query string, args ...any) ([]Summary, error) {
	rows, err := x.db.Query(query, args...)
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

func (x *Index) summary(id string) (Summary, error) {
	return scanSummary(x.db.QueryRow(`SELECT `+summaryCols+` FROM nodes n WHERE n.id = ?`, id))
}

// Initiative is an initiative in one line, with its slug, which is "" when
// the record has none.
type Initiative struct {
	Summary
	Slug string
}

// Initiatives returns every initiative with its slug, by ID, including those
// whose slug is empty or shared.
func (x *Index) Initiatives() ([]Initiative, error) {
	rows, err := x.db.Query(`SELECT ` + summaryCols + `, i.slug FROM nodes n JOIN initiatives i ON i.id = n.id ORDER BY n.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Initiative
	for rows.Next() {
		var i Initiative
		if err := rows.Scan(&i.ID, &i.Type, &i.Title, &i.Status, &i.Slug); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Where returns where the initiative stands, as View says. name is its slug,
// its ID or an alias. An unknown name gives a *NotFoundError, the name of another kind of
// record a *NotInitiativeError, and a slug two initiatives share a
// *DuplicateSlugError.
func (x *Index) Where(name string) (*View, error) {
	id, err := x.initiative(name)
	if err != nil {
		return nil, err
	}
	v := &View{}
	if v.Initiative, err = x.summary(id); err != nil {
		return nil, err
	}
	if err := x.db.QueryRow(`SELECT slug FROM initiatives WHERE id = ?`, id).Scan(&v.Slug); err != nil {
		return nil, err
	}
	if v.InProgress, err = x.summaries(`SELECT `+summaryCols+` FROM nodes n JOIN members m ON m.id = n.id
		WHERE m.initiative = ? AND n.progress = 'in_progress' ORDER BY n.id`, id); err != nil {
		return nil, err
	}
	if v.Blocked, err = x.blocked(id); err != nil {
		return nil, err
	}
	if v.Open, err = x.openQuestions(id); err != nil {
		return nil, err
	}
	if v.Recheck, err = x.recheck(id); err != nil {
		return nil, err
	}
	return v, nil
}

// blocked returns the initiative's blocked tasks, with what each waits on.
func (x *Index) blocked(ini string) ([]Blocked, error) {
	tasks, err := x.summaries(`SELECT `+summaryCols+` FROM nodes n JOIN members m ON m.id = n.id
		WHERE m.initiative = ? AND n.progress = 'blocked' ORDER BY n.id`, ini)
	if err != nil {
		return nil, err
	}
	var out []Blocked
	for _, t := range tasks {
		by, err := x.summaries(`SELECT DISTINCT `+summaryCols+` FROM edges e JOIN nodes n ON n.id = e.dst
			WHERE e.src = ? AND e.name = 'blocked_by' ORDER BY n.id`, t.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, Blocked{Task: t, By: by})
	}
	return out, nil
}

// openQuestions returns the initiative's questions that nothing settles, and
// those that something settles while another answer still waits, with their
// pending answers.
func (x *Index) openQuestions(ini string) ([]Question, error) {
	qs, err := x.summaries(`SELECT `+summaryCols+` FROM nodes n JOIN members m ON m.id = n.id
		WHERE m.initiative = ? AND n.type = 'question' AND (n.settled = 0 OR EXISTS (
			SELECT 1 FROM edges e WHERE e.dst = n.id AND e.name = 'answers' AND e.pending = 1)) ORDER BY n.id`, ini)
	if err != nil {
		return nil, err
	}
	var out []Question
	for _, q := range qs {
		pending, err := x.summaries(`SELECT DISTINCT `+summaryCols+` FROM edges e JOIN nodes n ON n.id = e.src
			WHERE e.dst = ? AND e.name = 'answers' AND e.pending = 1 ORDER BY n.id`, q.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, Question{Summary: q, Pending: pending})
	}
	return out, nil
}

// recheck returns the links of the initiative's members, and of their
// sub-records, that are stale, reopen their source, rest on a record no
// longer in force or rely on a doubted belief, grouped by the record that
// changed.
func (x *Index) recheck(ini string) ([]Change, error) {
	rows, err := x.db.Query(`SELECT t.id, t.type, t.title, t.status, COALESCE(t.revision, 0),
		e.name, e.pinned_rev, e.pinned_fp, e.stale, e.reopens, e.orphaned, e.doubted, s.id, s.type, s.title, s.status
		FROM edges e JOIN members m ON m.id = e.src JOIN nodes t ON t.id = e.dst JOIN nodes s ON s.id = e.src
		WHERE m.initiative = ? AND (e.stale = 1 OR e.reopens = 1 OR e.orphaned = 1 OR e.doubted = 1) ORDER BY t.id, s.id, e.name`, ini)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var t Summary
		var rev int
		var l RecheckLink
		var pinRev sql.NullInt64
		var fp string
		if err := rows.Scan(&t.ID, &t.Type, &t.Title, &t.Status, &rev,
			&l.Field, &pinRev, &fp, &l.Stale, &l.Reopens, &l.Orphaned, &l.Doubted, &l.Other.ID, &l.Other.Type, &l.Other.Title, &l.Other.Status); err != nil {
			return nil, err
		}
		if pinRev.Valid {
			l.Pin = &Pin{Rev: int(pinRev.Int64), FP: fp}
		}
		if n := len(out); n == 0 || out[n-1].Target.ID != t.ID {
			out = append(out, Change{Target: t, Revision: rev})
		}
		c := &out[len(out)-1]
		c.Links = append(c.Links, l)
	}
	return out, rows.Err()
}

// Overview returns what the README of the initiative with the slug shows. An
// unknown slug gives a *NotFoundError, and a slug two initiatives share a
// *DuplicateSlugError naming both, so a caller can report that neither gets
// a page.
func (x *Index) Overview(slug string) (*Overview, error) {
	id, err := x.bySlug(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, x.notFoundIn(slug, slugsAndIDs)
	}
	o := &Overview{Slug: slug}
	var goals string
	if err := x.db.QueryRow(`SELECT `+summaryCols+`, n.path, n.body, i.purpose, i.goals
		FROM nodes n JOIN initiatives i ON i.id = n.id WHERE n.id = ?`, id).Scan(
		&o.Initiative.ID, &o.Initiative.Type, &o.Initiative.Title, &o.Initiative.Status,
		&o.Path, &o.Body, &o.Purpose, &goals); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(goals), &o.Goals); err != nil {
		return nil, fmt.Errorf("%s: goals: %w", id, err)
	}
	if o.People, err = x.people(id); err != nil {
		return nil, err
	}
	for _, list := range []struct {
		typ string
		to  *[]Entry
	}{{"design_doc", &o.DesignDocs}, {"prd", &o.PRDs}, {"task", &o.Tasks}} {
		if *list.to, err = x.entries(id, list.typ); err != nil {
			return nil, err
		}
	}
	if o.Findings, err = x.findings(id); err != nil {
		return nil, err
	}
	return o, nil
}

// findings returns the findings produced in the initiative's activities, by
// title (ignoring ASCII case) then ID.
func (x *Index) findings(ini string) ([]Entry, error) {
	rows, err := x.db.Query(`SELECT `+summaryCols+`, n.path, n.progress FROM nodes n JOIN produced p ON p.id = n.id
		WHERE p.initiative = ? AND n.type = 'finding' ORDER BY n.title COLLATE NOCASE, n.id`, ini)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Type, &e.Title, &e.Status, &e.Path, &e.Progress); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// people returns the actors the initiative involves, in the order its record
// lists them: the links were stored in that order.
func (x *Index) people(ini string) ([]Person, error) {
	rows, err := x.db.Query(`SELECT `+summaryCols+`, e.role FROM edges e JOIN nodes n ON n.id = e.dst
		WHERE e.src = ? AND e.name = 'involves' ORDER BY e.rowid`, ini)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Type, &p.Title, &p.Status, &p.Role); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// entries returns the initiative's members of one type, by title (ignoring
// ASCII case, as SQLite's NOCASE does) then ID.
func (x *Index) entries(ini, typ string) ([]Entry, error) {
	rows, err := x.db.Query(`SELECT `+summaryCols+`, n.path, n.progress FROM nodes n JOIN members m ON m.id = n.id
		WHERE m.initiative = ? AND n.type = ? ORDER BY n.title COLLATE NOCASE, n.id`, ini, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Type, &e.Title, &e.Status, &e.Path, &e.Progress); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ResolveInitiative returns the initiative that name stands for, with its
// slug. The name may be a slug, an ID or an alias, as in Where, and the
// errors are Where's.
func (x *Index) ResolveInitiative(name string) (Initiative, error) {
	id, err := x.initiative(name)
	if err != nil {
		return Initiative{}, err
	}
	s, err := x.summary(id)
	if err != nil {
		return Initiative{}, err
	}
	ini := Initiative{Summary: s}
	err = x.db.QueryRow(`SELECT slug FROM initiatives WHERE id = ?`, id).Scan(&ini.Slug)
	return ini, err
}
