// Package index keeps a SQLite copy of the whole workspace, so a query
// reads one file instead of loading and checking every record. The copy
// holds the graph, what the checks found (as a count), and a signature of
// the files it was built from. It is rebuilt from graph/ whenever the files,
// the loremason version, the fingerprint method, the index's own table layout
// (SchemaVersion) or the record types (every embedded schema file) have
// changed, so there is no incremental update to get wrong. Several processes
// may open the same workspace at once: each builds into its own temporary
// file and renames it into place, and any of the results is as good as
// another. When .loremason can't be written to (a read-only checkout), the
// index is built in a temporary file outside the workspace and removed on
// Close. Whether a link is stale or reopens a commitment, a task's progress,
// whether a question is settled, and which records belong to an initiative
// are decided in package graph and stored; the queries here read what is
// stored, and walk the stored links to find dependents.
//
// The design is in docs/context-graph/03-implementation.md, section 6.
package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	_ "modernc.org/sqlite" // the pure-Go SQLite driver

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/workspace"
	"github.com/andycondon/loremason/schema"
)

const (
	dir      = ".loremason"
	fileName = "index.db"
	ignore   = "index.db*\n" // the index, and the temporary and journal files beside it
)

// SchemaVersion is the version of the index's own tables. Raise it whenever
// the tables or what is stored in them change, so older indexes are rebuilt.
// It isn't the loremason version in the workspace's config; that is
// Meta.Version.
const SchemaVersion = 2

// Index is an open index. Close it when done.
type Index struct {
	db       *sql.DB
	problems int
	rebuilt  bool
	temp     string // an index built outside the workspace, removed on Close; "" otherwise
}

// Meta is what an index records about how it was built.
type Meta struct {
	Version           string // the workspace config's loremason version
	Schema            int    // SchemaVersion, the version of the index's tables
	Types             string // a hash of every embedded schema file, types.yaml and the JSON schemas
	FingerprintMethod int    // fingerprint.Method
	Signature         string // of the files under graph/; see Signature
	Problems          int    // errors found loading and checking, not warnings
}

// Open returns the index of the workspace at root, building it first when
// there is none, it can't be read, or what it was built from differs from
// what is there now: the files under graph/, the loremason version, the
// fingerprint method, SchemaVersion or the record types (any embedded schema
// file). It also writes .loremason/.gitignore when that is missing, so the
// index isn't committed. When the index can't be written in .loremason, it
// is built in a temporary file outside the workspace instead, and Close
// removes it.
func Open(root string, reg *registry.Registry) (*Index, error) {
	sig, err := Signature(root)
	if err != nil {
		return nil, err
	}
	return openWith(root, sig, func() (*check.Result, error) { return check.Run(root, reg) })
}

// FromCheck opens the index of the workspace at root for a command that has
// already loaded and checked the graph, as Open does, but builds a stale
// index from res instead of loading the graph again. sig is the Signature
// taken before the graph was loaded, so a file edited while the command runs
// makes the next open rebuild. res is only read when the index has to be
// built.
func FromCheck(root string, res *check.Result, sig string) (*Index, error) {
	return openWith(root, sig, func() (*check.Result, error) { return res, nil })
}

// openWith returns the index of the workspace at root for the files signed sig,
// building it from what load returns when it is out of date.
func openWith(root, sig string, load func() (*check.Result, error)) (*Index, error) {
	ws, err := workspace.Find(root, root)
	if err != nil {
		return nil, err
	}
	ignoreErr := writeIgnore(root)
	want := Meta{Version: ws.Config.Version, Schema: SchemaVersion, Types: typesHash(), FingerprintMethod: fingerprint.Method, Signature: sig}
	path := filepath.Join(root, dir, fileName)

	rebuilt, temp := false, ""
	got, ok := readMeta(path)
	if !ok || !got.sameBuild(want) {
		res, err := load()
		if err != nil {
			return nil, err
		}
		errs, _ := res.Diagnostics.Count()
		want.Problems = errs
		if ignoreErr == nil {
			err = Build(path, res.Graph, want)
		}
		if ignoreErr != nil || notWritable(err) {
			path, err = buildOutside(res, want)
			temp = path
		}
		if err != nil {
			return nil, err
		}
		got, rebuilt = want, true
	}
	db, err := openDB(path)
	if err != nil {
		if temp != "" {
			os.Remove(temp)
		}
		return nil, err
	}
	return &Index{db: db, problems: got.Problems, rebuilt: rebuilt, temp: temp}, nil
}

// notWritable reports whether err says a file couldn't be created because
// of permissions or a read-only file system.
func notWritable(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS)
}

// buildOutside builds the index in a new temporary file outside the
// workspace, and returns its path.
func buildOutside(res *check.Result, m Meta) (string, error) {
	f, err := os.CreateTemp("", "loremason-index-*.db")
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()
	if err := Build(path, res.Graph, m); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// sameBuild reports whether an index with m was built the way want says,
// ignoring the problem count, which is a result of the build.
func (m Meta) sameBuild(want Meta) bool {
	m.Problems, want.Problems = 0, 0
	return m == want
}

// schemaFiles is where the record types are read from; a test stands in
// others.
var schemaFiles fs.FS = schema.FS

// typesHash returns a hash of the embedded record types: the name and bytes
// of every file in the schema folder (types.yaml and the JSON schemas), in
// name order. A field added to one record type's schema changes it as much
// as an edit to types.yaml.
func typesHash() string {
	h := sha256.New()
	err := fs.WalkDir(schemaFiles, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(schemaFiles, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\t%d\n", p, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		panic("the embedded schema can't be read: " + err.Error()) // the tests rule this out
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Close closes the index, and removes it when it was built outside the
// workspace.
func (x *Index) Close() error {
	err := x.db.Close()
	if x.temp != "" {
		os.Remove(x.temp)
	}
	return err
}

// Problems returns the number of errors found loading and checking the
// workspace when the index was built. A query warns when it isn't zero.
func (x *Index) Problems() int { return x.problems }

// Rebuilt reports whether Open built the index, rather than finding it
// current.
func (x *Index) Rebuilt() bool { return x.rebuilt }

// probeFTS5 reports why the SQLite build can't do full-text search, or nil.
// It's a variable so a test can stand in a build that can't.
var probeFTS5 = func(db *sql.DB) error {
	if _, err := db.Exec(`CREATE VIRTUAL TABLE temp.fts5_probe USING fts5(x)`); err != nil {
		return err
	}
	_, err := db.Exec(`DROP TABLE temp.fts5_probe`)
	return err
}

// errNoFTS5 is the error for a SQLite build without FTS5.
func errNoFTS5(cause error) error {
	return fmt.Errorf("full-text search (FTS5) is missing from this SQLite build, and mason needs it to find records by words (%v); install a mason release, which includes it, or rebuild mason with an SQLite driver that has FTS5", cause)
}

// busyTimeout is how long, in milliseconds, a connection waits for a lock
// another process holds before it gives up.
const busyTimeout = 10000

// connect opens an SQLite file that waits busyTimeout for locks. A relative
// path is made absolute first, since a file: URI can't hold one with "..".
func connect(path string) (*sql.DB, error) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=busy_timeout(" + strconv.Itoa(busyTimeout) + ")"}
	return sql.Open("sqlite", u.String())
}

// openDB opens an SQLite file and checks that it can do full-text search. A
// probe that fails for another reason, such as a busy file, is returned as it
// is.
func openDB(path string) (*sql.DB, error) {
	db, err := connect(path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // the temp probe table and the queries share a connection
	if err := probeFTS5(db); err != nil {
		db.Close()
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil, errNoFTS5(err)
		}
		return nil, err
	}
	return db, nil
}

// readMeta returns what the index at path says about how it was built. ok is
// false when there is no index, or it can't be read.
func readMeta(path string) (m Meta, ok bool) {
	if _, err := os.Stat(path); err != nil {
		return Meta{}, false
	}
	db, err := connect(path)
	if err != nil {
		return Meta{}, false
	}
	defer db.Close()
	rows, err := db.Query(`SELECT key, value FROM meta`)
	if err != nil {
		return Meta{}, false
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			return Meta{}, false
		}
		var perr error
		switch k {
		case "version":
			m.Version = v
		case "schema":
			m.Schema, perr = strconv.Atoi(v)
		case "types":
			m.Types = v
		case "fingerprint_method":
			m.FingerprintMethod, perr = strconv.Atoi(v)
		case "signature":
			m.Signature = v
		case "problems":
			m.Problems, perr = strconv.Atoi(v)
		default:
			continue
		}
		if perr != nil {
			return Meta{}, false
		}
		seen++
	}
	return m, rows.Err() == nil && seen == 6
}

// Signature summarises every file under root's graph/ folder: its path,
// size and modification time. An added, edited or deleted file changes it.
func Signature(root string) (string, error) {
	base, err := filepath.EvalSymlinks(filepath.Join(root, graph.Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	h := sha256.New()
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\t%d\t%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeIgnore writes .loremason/.gitignore, unless it is already there. It
// covers the index and every temporary or journal file beside it, but not
// config.yaml.
func writeIgnore(root string) error {
	p := filepath.Join(root, dir, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return os.WriteFile(p, []byte(ignore), 0o644)
}
