package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/andycondon/loremason/internal/graph"
)

const tables = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE nodes (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	status TEXT NOT NULL,
	progress TEXT NOT NULL,
	settled INTEGER NOT NULL,
	revision INTEGER,
	fingerprint TEXT NOT NULL,
	path TEXT NOT NULL,
	header TEXT NOT NULL,
	body TEXT NOT NULL
);
CREATE TABLE aliases (alias TEXT PRIMARY KEY, id TEXT NOT NULL);
CREATE TABLE edges (
	src TEXT NOT NULL,
	dst TEXT NOT NULL,
	name TEXT NOT NULL,
	pinned_rev INTEGER,
	pinned_fp TEXT NOT NULL,
	propagates INTEGER NOT NULL,
	stale INTEGER NOT NULL,
	reopens INTEGER NOT NULL,
	settles INTEGER NOT NULL,
	pending INTEGER NOT NULL,
	role TEXT NOT NULL
);
CREATE TABLE members (
	initiative TEXT NOT NULL,
	id TEXT NOT NULL,
	PRIMARY KEY (initiative, id)
);
CREATE INDEX members_id ON members (id);
CREATE TABLE produced (
	initiative TEXT NOT NULL,
	id TEXT NOT NULL,
	PRIMARY KEY (initiative, id)
);
CREATE TABLE initiatives (
	id TEXT PRIMARY KEY,
	slug TEXT NOT NULL,
	purpose TEXT NOT NULL,
	goals TEXT NOT NULL
);
CREATE INDEX edges_dst ON edges (dst);
CREATE INDEX edges_src ON edges (src);
CREATE VIRTUAL TABLE nodes_fts USING fts5(id UNINDEXED, title, body);
`

// Containment is the name of the edge from a sub-record to the record
// holding it. It doesn't propagate: a change to a sub-record is a change to
// its record, and links pin that record's revisions. Show leaves it out and
// reports the sub-record's links with the record instead.
const Containment = "subrecord_of"

// Build writes an index of g, recording m, to path. Everything is written,
// in one transaction, to a new temporary file beside path (named so
// .gitignore covers it), and the file is then renamed into place, so a
// reader never sees half an index. Builds running at once each have their
// own file; the last to finish is the one left. Each record and sub-record
// gets a row in nodes, with the progress and whether it is settled as the
// graph works them out, and each record in nodes_fts for text search. Each
// alias gets a row in aliases. Each link that reaches a record, and each
// sub-record's containment in its record, gets a row in edges, with whether
// it is stale, reopens its source, settles its target or is a pending
// answer, and its role. Each initiative gets a row in initiatives and a row
// in members for each record in it, and a row in produced for each record
// produced in its activities. A second file with an ID the graph already has
// is left out.
func Build(path string, g *graph.Graph, m Meta) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	db, err := openDB(tmp)
	if err != nil {
		return err
	}
	if err = write(db, g, m); err != nil {
		db.Close()
		return err
	}
	if err = db.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// write puts the tables, the meta rows and everything in g into db, in one
// transaction.
func write(db *sql.DB, g *graph.Graph, m Meta) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(tables); err != nil {
		return err
	}
	meta := [][2]string{
		{"version", m.Version},
		{"schema", strconv.Itoa(m.Schema)},
		{"types", m.Types},
		{"fingerprint_method", strconv.Itoa(m.FingerprintMethod)},
		{"signature", m.Signature},
		{"problems", strconv.Itoa(m.Problems)},
	}
	for _, kv := range meta {
		if _, err := tx.Exec(`INSERT INTO meta VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	w := &writer{g: g, tx: tx}
	if err := w.nodes(); err != nil {
		return err
	}
	if err := w.aliases(); err != nil {
		return err
	}
	if err := w.edges(); err != nil {
		return err
	}
	if err := w.initiatives(); err != nil {
		return err
	}
	return tx.Commit()
}

type writer struct {
	g  *graph.Graph
	tx *sql.Tx
}

// canonical reports whether n is the node the graph knows by its ID. A
// second file with the same ID is in the graph's files but not its names, so
// it, and what it links to, is left out.
func (w *writer) canonical(n *graph.Node) bool { return w.g.Node(n.ID) == n }

func (w *writer) nodes() error {
	node, err := w.tx.Prepare(`INSERT INTO nodes VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer node.Close()
	fts, err := w.tx.Prepare(`INSERT INTO nodes_fts (id, title, body) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer fts.Close()
	for _, f := range w.g.Files {
		for _, n := range append([]*graph.Node{f.Node}, f.Node.Children...) {
			if !w.canonical(n) {
				continue
			}
			title, body, header := n.Fields.Text("title"), "", ""
			if title == "" {
				title = n.ID
			}
			if n.File.Record != nil && !n.IsSubrecord() {
				body = n.File.Record.Body
				data, err := json.Marshal(n.File.Record.Data) // sorted keys, so the same on every build
				if err != nil {
					return fmt.Errorf("%s: %w", n.File.Path, err)
				}
				header = string(data)
			}
			var rev any
			if c := n.Current(); c != nil {
				rev = c.Rev
			}
			fp := ""
			if v := n.Fingerprint(); !v.IsZero() {
				fp = v.String()
			}
			if _, err := node.Exec(n.ID, n.Kind.Name, title, n.Status(), n.Progress(), n.Settled(), rev, fp, n.File.Path, header, body); err != nil {
				return err
			}
			if !n.IsSubrecord() {
				if _, err := fts.Exec(n.ID, title, body); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// aliases writes each alias, as the graph resolves it.
func (w *writer) aliases() error {
	for _, name := range w.g.Names() {
		if w.g.Node(name) != nil {
			continue
		}
		n := w.g.Resolve(graph.Ref{Name: name})
		if n == nil || !w.canonical(n) {
			continue
		}
		if _, err := w.tx.Exec(`INSERT INTO aliases VALUES (?, ?)`, name, n.ID); err != nil {
			return err
		}
	}
	return nil
}

// edges writes every link that reaches a record, with the answers the graph
// gives for it, and the containment of each sub-record.
func (w *writer) edges() error {
	ins, err := w.tx.Prepare(`INSERT INTO edges VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, e := range w.g.Edges {
		if e.Target == nil || !w.canonical(e.From) || !w.canonical(e.Target) {
			continue
		}
		var rev any
		fp := ""
		if e.Pin != nil {
			rev = e.Pin.Rev
			if !e.Pin.FP.IsZero() {
				fp = e.Pin.FP.String()
			}
		}
		if _, err := ins.Exec(e.From.ID, e.Target.ID, e.Name, rev, fp, e.Def.Propagates, e.Stale(), e.Reopens(), e.Settles(), e.Pending(), e.Attr("role")); err != nil {
			return err
		}
	}
	for _, f := range w.g.Files {
		if !w.canonical(f.Node) {
			continue
		}
		for _, c := range f.Node.Children {
			if !w.canonical(c) {
				continue
			}
			if _, err := ins.Exec(c.ID, f.Node.ID, Containment, nil, "", false, false, false, false, false, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// initiatives writes each initiative's slug, purpose and goals, and the
// records in it and produced in its activities, as the graph works them out.
func (w *writer) initiatives() error {
	ini, err := w.tx.Prepare(`INSERT INTO initiatives VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ini.Close()
	member, err := w.tx.Prepare(`INSERT INTO members VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer member.Close()
	made, err := w.tx.Prepare(`INSERT INTO produced VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer made.Close()
	for _, f := range w.g.Files {
		n := f.Node
		if n.Kind.Name != "initiative" || !w.canonical(n) {
			continue
		}
		goals := []string{} // a list even when there are none
		if v, ok := n.Fields.Get("goals"); ok {
			for _, item := range v.Items() {
				if s, ok := item.Text(); ok {
					goals = append(goals, s)
				}
			}
		}
		data, err := json.Marshal(goals)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		if _, err := ini.Exec(n.ID, n.Fields.Text("slug"), n.Fields.Text("purpose"), string(data)); err != nil {
			return err
		}
		for _, m := range w.g.Members(n) {
			if !w.canonical(m) {
				continue
			}
			if _, err := member.Exec(n.ID, m.ID); err != nil {
				return err
			}
		}
		for _, m := range w.g.Produced(n) {
			if !w.canonical(m) {
				continue
			}
			if _, err := made.Exec(n.ID, m.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
