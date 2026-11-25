// Package check validates every record in a graph folder. It only reads.
package check

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// Problem is one thing wrong with a file.
type Problem struct {
	Path    string // relative to the workspace, with forward slashes
	Line    int
	Warning bool
	Msg     string
}

func (p Problem) String() string {
	level := "error"
	if p.Warning {
		level = "warning"
	}
	return fmt.Sprintf("%s:%d: %s: %s", p.Path, p.Line, level, p.Msg)
}

// Result is what a check found.
type Result struct {
	Records  int
	Problems []Problem
}

// Count returns the number of errors and warnings.
func (r *Result) Count() (errs, warnings int) {
	for _, p := range r.Problems {
		if p.Warning {
			warnings++
		} else {
			errs++
		}
	}
	return
}

// file is one record file. rec is nil when it couldn't be read.
type file struct {
	path   string
	id     string         // from the filename
	typ    *registry.Type // from the ID's prefix, or else the folder
	folder *registry.Type
	rec    *record.Record
	bad    map[int]bool // lines the reader already reported
}

type target struct {
	typ string
	in  *file
}

type checker struct {
	reg     *registry.Registry
	files   []*file
	byID    map[string]target // records and sub-records
	aliases map[string]*file
	names   []string // every ID and alias, for suggestions
	res     Result
}

// Run checks the workspace at root.
func Run(root string, reg *registry.Registry) (*Result, error) {
	c := &checker{reg: reg, byID: map[string]target{}, aliases: map[string]*file{}}
	if err := c.load(root); err != nil {
		return nil, err
	}
	c.index()
	for _, f := range c.files {
		c.identify(f)
		if f.rec != nil {
			c.shape(f)
			c.links(f)
		}
	}
	c.duplicates()
	slices.SortStableFunc(c.res.Problems, func(a, b Problem) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Msg, b.Msg))
	})
	return &c.res, nil
}

func (c *checker) errorf(p string, line int, format string, args ...any) {
	c.res.Problems = append(c.res.Problems, Problem{Path: p, Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (c *checker) warnf(p string, line int, format string, args ...any) {
	c.res.Problems = append(c.res.Problems, Problem{Path: p, Line: line, Warning: true, Msg: fmt.Sprintf(format, args...)})
}

// didYouMean ends a message with a suggestion, when there is one.
func didYouMean(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("; did you mean `%s`?", s)
}

// load reads every file under graph/.
func (c *checker) load(root string) error {
	graph, err := filepath.EvalSymlinks(filepath.Join(root, "graph"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a new workspace has no records yet
	} else if err != nil {
		return err
	}
	var folders []string
	for _, t := range c.reg.Types {
		folders = append(folders, t.Folder())
	}
	slices.Sort(folders)
	return filepath.WalkDir(graph, func(p string, d fs.DirEntry, err error) error {
		if p == graph {
			return err
		}
		rel, relErr := filepath.Rel(graph, p)
		if relErr != nil {
			return relErr
		}
		rel = "graph/" + filepath.ToSlash(rel)
		if err != nil {
			c.errorf(rel, 1, "can't read: %v", unwrap(err))
			return nil
		}
		hidden := strings.HasPrefix(d.Name(), ".")
		parts := strings.Split(rel, "/")
		if d.IsDir() {
			if hidden {
				return fs.SkipDir
			}
			if len(parts) > 2 {
				c.errorf(rel, 1, "records aren't kept in folders inside a type's folder")
				return fs.SkipDir
			}
			if c.reg.ByFolder(d.Name()) == nil {
				c.errorf(rel, 1, "no record type has the folder `%s`%s", d.Name(), didYouMean(suggest.Near(d.Name(), folders)))
				return fs.SkipDir
			}
			return nil
		}
		if hidden {
			return nil
		}
		if len(parts) == 2 {
			c.errorf(rel, 1, "records are kept in a folder named after their type, such as graph/belief/")
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				d = fs.FileInfoToDirEntry(info)
			}
		}
		if !d.Type().IsRegular() || path.Ext(rel) != ".md" {
			c.errorf(rel, 1, "only record files (<id>.md) belong here")
			return nil
		}
		f := &file{path: rel, id: strings.TrimSuffix(d.Name(), ".md"), folder: c.reg.ByFolder(parts[1])}
		f.typ = f.folder
		if prefix, ok := id.Prefix(f.id); ok && c.reg.ByPrefix(prefix) != nil {
			f.typ = c.reg.ByPrefix(prefix)
		}
		c.files = append(c.files, f)
		c.res.Records++
		// A file that can't be read still has an ID, so links to it resolve.
		data, err := os.ReadFile(p)
		if err != nil {
			c.errorf(rel, 1, "can't read: %v", unwrap(err))
			return nil
		}
		rec, errs := record.Parse(data)
		f.bad = map[int]bool{}
		for _, e := range errs {
			c.errorf(rel, e.Line, "%s", e.Msg)
			f.bad[e.Line] = true
		}
		f.rec = rec
		return nil
	})
}

// unwrap drops the path from a file error; the message already names the file.
func unwrap(err error) error {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err
	}
	return err
}

// index notes every ID, sub-record and alias, so links can be followed.
// A record goes by its filename, so one that can't be read still resolves.
func (c *checker) index() {
	for _, f := range c.files {
		if first, ok := c.byID[f.id]; ok {
			c.errorf(f.path, 1, "`%s` is also the ID of %s", f.id, first.in.path)
		} else {
			c.byID[f.id] = target{f.typ.Name, f}
			c.names = append(c.names, f.id)
		}
		if f.rec == nil {
			continue
		}
		seen := map[string]bool{}
		for field, sub := range f.typ.Subrecords {
			list, _ := f.rec.Fields[field].([]any)
			for i, item := range list {
				entry, _ := item.(map[string]any)
				local, _ := entry["id"].(string)
				if local == "" {
					continue
				}
				if seen[local] {
					c.errorf(f.path, f.rec.Line(field, fmt.Sprint(i), "id"), "two entries have the id `%s`", local)
					continue
				}
				seen[local] = true
				full := f.id + "/" + local
				if _, ok := c.byID[full]; !ok {
					c.byID[full] = target{sub.Type, f}
					c.names = append(c.names, full)
				}
			}
		}
	}
	for _, f := range c.files {
		if f.rec == nil {
			continue
		}
		list, _ := f.rec.Fields["aliases"].([]any)
		for i, item := range list {
			alias, _ := item.(string)
			if alias == "" {
				continue
			}
			line := f.rec.Line("aliases", fmt.Sprint(i))
			if other, ok := c.byID[alias]; ok {
				c.errorf(f.path, line, "the alias `%s` is the ID of a record in %s", alias, other.in.path)
			} else if other := c.aliases[alias]; other != nil && other != f {
				c.errorf(f.path, line, "the alias `%s` is already used by %s", alias, other.path)
			} else if other == nil {
				c.aliases[alias] = f
				c.names = append(c.names, alias)
			}
		}
	}
	slices.Sort(c.names)
}

// identify checks that the id, filename and folder agree.
func (c *checker) identify(f *file) {
	if prefix, ok := id.Prefix(f.id); !ok {
		c.errorf(f.path, 1, "the filename must be an ID such as %s.md", f.folder.Prefix+"_7k3m9q")
	} else if t := c.reg.ByPrefix(prefix); t == nil {
		c.errorf(f.path, 1, "no record type has the prefix `%s_`; records in this folder start with `%s_`", prefix, f.folder.Prefix)
	} else if t != f.folder {
		c.errorf(f.path, 1, "`%s_` is the prefix of type %s, so this file belongs in graph/%s/", prefix, t.Name, t.Folder())
	}
	if f.rec == nil {
		return
	}
	if got, ok := f.rec.Fields["id"].(string); ok && got != f.id {
		c.errorf(f.path, f.rec.Line("id"), "the id `%s` doesn't match the filename `%s.md`", got, f.id)
	}
}

// duplicates warns about records that look like the same thing.
func (c *checker) duplicates() {
	seen := map[string]*file{}
	for _, f := range c.files {
		if f.rec == nil || len(f.typ.Identity) == 0 {
			continue
		}
		key := []string{f.typ.Name}
		for _, field := range f.typ.Identity {
			s, _ := f.rec.Fields[field].(string)
			if s = strings.ToLower(strings.Join(strings.Fields(s), " ")); s != "" {
				key = append(key, s)
			}
		}
		if len(key) <= len(f.typ.Identity) {
			continue // a field is empty, so there's nothing to compare
		}
		k := strings.Join(key, "\x00")
		if first := seen[k]; first != nil {
			c.warnf(f.path, f.rec.Line(f.typ.Identity[0]), "same %s as %s; if they're the same thing, use that record instead", strings.Join(f.typ.Identity, " and "), first.id)
		} else {
			seen[k] = f
		}
	}
}
