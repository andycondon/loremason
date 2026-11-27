// Package index keeps a SQLite copy of the whole workspace, so a query
// reads one file instead of loading and checking every record. The copy
// holds the graph, what the checks found (as a count), and a signature of
// the files it was built from. It is rebuilt from graph/ whenever the files,
// the loremason version, the fingerprint method, the index's own table layout
// (SchemaVersion) or the record types (types.yaml) have changed, so there is
// no incremental update to get wrong. Several processes may open the same
// workspace at once: each builds into its own temporary file and renames it
// into place, and any of the results is as good as another. Whether a link
// is stale or reopens a commitment is decided in package graph and stored;
// the queries here only select and join.
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
const SchemaVersion = 1

// Index is an open index. Close it when done.
type Index struct {
	db       *sql.DB
	problems int
	rebuilt  bool
}

// Meta is what an index records about how it was built.
type Meta struct {
	Version           string // the loremason version in the workspace's config, not the index's layout
	Schema            int    // SchemaVersion, the version of the index's tables
	Types             string // a hash of the embedded schema/types.yaml, so a change to the record types rebuilds
	FingerprintMethod int    // fingerprint.Method
	Signature         string // of the files under graph/; see Signature
	Problems          int    // errors found loading and checking, not counting warnings
}

// Open returns the index of the workspace at root, building it first when
// there is none or the files under graph/, the loremason version or the
// fingerprint method differ from what it was built from. It also writes
// .loremason/.gitignore when that is missing, so the index isn't committed.
func Open(root string, reg *registry.Registry) (*Index, error) {
	ws, err := workspace.Find(root, root)
	if err != nil {
		return nil, err
	}
	if err := writeIgnore(root); err != nil {
		return nil, err
	}
	sig, err := Signature(root)
	if err != nil {
		return nil, err
	}
	want := Meta{Version: ws.Config.Version, Schema: SchemaVersion, Types: typesHash(), FingerprintMethod: fingerprint.Method, Signature: sig}
	path := filepath.Join(root, dir, fileName)

	rebuilt := false
	got, ok := readMeta(path)
	if !ok || !got.sameBuild(want) {
		res, err := check.Run(root, reg)
		if err != nil {
			return nil, err
		}
		errs, _ := res.Diagnostics.Count()
		want.Problems = errs
		if err := Build(path, res.Graph, want); err != nil {
			return nil, err
		}
		got, rebuilt = want, true
	}
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	return &Index{db: db, problems: got.Problems, rebuilt: rebuilt}, nil
}

// sameBuild reports whether an index with m was built the way want says,
// ignoring the problem count, which is a result of the build.
func (m Meta) sameBuild(want Meta) bool {
	m.Problems, want.Problems = 0, 0
	return m == want
}

// typesHash returns a hash of the embedded record types.
func typesHash() string {
	data, err := schema.FS.ReadFile("types.yaml")
	if err != nil {
		panic("the embedded types.yaml is missing: " + err.Error()) // the tests rule this out
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Close closes the index.
func (x *Index) Close() error { return x.db.Close() }

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

// connect opens an SQLite file that waits busyTimeout for locks.
func connect(path string) (*sql.DB, error) {
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

// writeIgnore writes .loremason/.gitignore, covering the index and every
// temporary or journal file beside it but not config.yaml, unless
// the file is already there.
func writeIgnore(root string) error {
	p := filepath.Join(root, dir, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return os.WriteFile(p, []byte(ignore), 0o644)
}
