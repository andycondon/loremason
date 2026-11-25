package graph

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// Load reads every record under root/graph and links them up. Problems that
// stop a file or link from being part of the graph go in the returned list;
// the graph holds everything else. A file that can't be read is still a node,
// under the ID its filename gives, so links to it resolve.
func Load(root string, reg *registry.Registry) (*Graph, diag.List, error) {
	l := &loader{reg: reg, g: &Graph{nodes: map[string]*Node{}, aliases: map[string]*Node{}}}
	if err := l.walk(root); err != nil {
		return nil, nil, err
	}
	for _, f := range l.g.Files {
		l.addRecord(f)
	}
	for _, f := range l.g.Files {
		l.addAliases(f)
	}
	for _, f := range l.g.Files {
		l.addEdges(f)
	}
	return l.g, l.ds, nil
}

type loader struct {
	reg *registry.Registry
	g   *Graph
	ds  diag.List
}

// walk reads the files under graph/, checking that each is where a record belongs.
func (l *loader) walk(root string) error {
	dir, err := filepath.EvalSymlinks(filepath.Join(root, "graph"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a new workspace has no records yet
	} else if err != nil {
		return err
	}
	var folders []string
	for _, t := range l.reg.Types {
		folders = append(folders, t.Folder())
	}
	slices.Sort(folders)
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if p == dir {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1 // 1 for type folders, 2 for records
		rel = "graph/" + filepath.ToSlash(rel)
		if err != nil {
			l.ds.Errorf(rel, 1, "can't read: %v", unwrap(err))
			return nil
		}
		hidden := strings.HasPrefix(d.Name(), ".")
		if d.IsDir() {
			switch {
			case hidden:
			case depth > 1:
				l.ds.Errorf(rel, 1, "records aren't kept in folders inside a type's folder")
			case l.reg.ByFolder(d.Name()) == nil:
				l.ds.Errorf(rel, 1, "no record type has the folder `%s`%s", d.Name(), suggest.Hint(suggest.Near(d.Name(), folders)))
			default:
				return nil
			}
			return fs.SkipDir
		}
		if hidden {
			return nil
		}
		if depth == 1 {
			l.ds.Errorf(rel, 1, "records are kept in a folder named after their type, such as graph/belief/")
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				d = fs.FileInfoToDirEntry(info)
			}
		}
		if !d.Type().IsRegular() || path.Ext(rel) != ".md" {
			l.ds.Errorf(rel, 1, "only record files (<id>.md) belong here")
			return nil
		}
		l.read(p, rel, l.reg.ByFolder(filepath.Base(filepath.Dir(p))))
		return nil
	})
}

// read adds one file, parsing it if it can.
func (l *loader) read(p, rel string, folder *registry.Type) {
	f := &File{Path: rel, Folder: folder, Type: folder}
	rid := strings.TrimSuffix(path.Base(rel), ".md")
	if prefix, ok := id.Prefix(rid); ok && l.reg.ByPrefix(prefix) != nil {
		f.Type = l.reg.ByPrefix(prefix)
	}
	f.Node = &Node{ID: rid, Type: f.Type.Name, File: f, Line: 1}
	l.g.Files = append(l.g.Files, f)
	data, err := os.ReadFile(p)
	if err != nil {
		l.ds.Errorf(rel, 1, "can't read: %v", unwrap(err))
		return
	}
	rec, errs := record.Parse(data)
	f.Record, f.Bad = rec, map[int]bool{}
	for _, e := range errs {
		l.ds.Errorf(rel, e.Line, "%s", e.Msg)
		f.Bad[e.Line] = true
	}
}

// addRecord indexes a file's record and its sub-records.
func (l *loader) addRecord(f *File) {
	n := f.Node
	if first := l.g.nodes[n.ID]; first != nil {
		l.ds.Errorf(f.Path, 1, "`%s` is also the ID of %s", n.ID, first.File.Path)
	} else {
		l.g.nodes[n.ID] = n
	}
	for _, field := range slices.Sorted(maps.Keys(f.Type.Subrecords)) {
		sub := f.Type.Subrecords[field]
		for _, entry := range l.entries(f, field) {
			local := record.Text(entry, "id")
			if local == "" {
				continue // the schema says what's missing
			}
			if n.Child(local) != nil {
				key, _ := record.Get(entry, "id")
				l.ds.Errorf(f.Path, key.Line, "two entries have the id `%s`", local)
				continue
			}
			c := &Node{ID: n.ID + "/" + local, Type: sub.Type, File: f, Parent: n, Local: local, Line: entry.Line}
			n.Children = append(n.Children, c)
			if l.g.nodes[c.ID] == nil {
				l.g.nodes[c.ID] = c
			}
		}
	}
}

// entries returns the mapping nodes in a list field.
func (l *loader) entries(f *File, field string) []*yaml.Node {
	if f.Record == nil {
		return nil
	}
	_, list := record.Get(f.Record.Header, field)
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	var out []*yaml.Node
	for _, item := range list.Content {
		if item.Kind == yaml.MappingNode {
			out = append(out, item)
		}
	}
	return out
}

// addAliases indexes a record's other names.
func (l *loader) addAliases(f *File) {
	if f.Record == nil {
		return
	}
	_, list := record.Get(f.Record.Header, "aliases")
	if list == nil || list.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range list.Content {
		alias := item.Value
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || alias == "" {
			continue
		}
		if other := l.g.nodes[alias]; other != nil {
			l.ds.Errorf(f.Path, item.Line, "the alias `%s` is the ID of a record in %s", alias, other.File.Path)
		} else if other := l.g.aliases[alias]; other != nil && other != f.Node {
			l.ds.Errorf(f.Path, item.Line, "the alias `%s` is already used by %s", alias, other.File.Path)
		} else {
			l.g.aliases[alias] = f.Node
		}
	}
}

// viaDef is the edge from a pin to the evidence for its last re-check.
var viaDef = registry.Edge{To: []string{"evidence"}}

// addEdges reads the links a record and its sub-records hold. A value of the
// wrong shape is skipped; the schema check reports it.
func (l *loader) addEdges(f *File) {
	if f.Record == nil {
		return
	}
	l.linksIn(f.Node, f.Record.Header, f.Type.Edges, "")
	for _, field := range slices.Sorted(maps.Keys(f.Type.Subrecords)) {
		sub := f.Type.Subrecords[field]
		for i, entry := range l.entries(f, field) {
			if c := f.Node.Child(record.Text(entry, "id")); c != nil && c.Line == entry.Line {
				l.linksIn(c, entry, sub.Edges, fmt.Sprintf("%s[%d].", field, i))
			}
		}
	}
}

// linksIn reads the edge fields of one mapping node, in sorted field order.
func (l *loader) linksIn(from *Node, m *yaml.Node, defs map[string]registry.Edge, prefix string) {
	for _, name := range slices.Sorted(maps.Keys(defs)) {
		_, v := record.Get(m, name)
		if v == nil {
			continue
		}
		field := prefix + name
		if v.Kind != yaml.SequenceNode {
			l.link(from, name, defs[name], v, field)
			continue
		}
		for i, item := range v.Content {
			l.link(from, name, defs[name], item, fmt.Sprintf("%s[%d]", field, i))
		}
	}
}

// link reads one link: an ID, or a map with `to` and perhaps a pin.
func (l *loader) link(from *Node, name string, def registry.Edge, v *yaml.Node, field string) {
	e := &Edge{From: from, Name: name, Def: def}
	var ok bool
	switch v.Kind {
	case yaml.ScalarNode:
		e.To, e.At, ok = l.ref(from, v, field)
	case yaml.MappingNode:
		_, to := record.Get(v, "to")
		if e.To, e.At, ok = l.ref(from, to, field+".to"); !ok {
			return
		}
		_, rev := record.Get(v, "rev")
		_, fp := record.Get(v, "fp")
		if rev != nil || fp != nil {
			e.Pin = &Pin{FP: record.Text(v, "fp")}
			if rev != nil {
				rev.Decode(&e.Pin.Rev) // a bad value is the schema's to report
			}
			if _, via := record.Get(v, "via"); via != nil {
				if to, at, ok := l.ref(from, via, field+".via"); ok {
					e.Pin.Via = &Edge{From: from, Name: "via", Def: viaDef, To: to, At: at}
				}
			}
		}
	}
	if !ok {
		return
	}
	l.g.Edges = append(l.g.Edges, e)
	if e.Pin != nil && e.Pin.Via != nil {
		l.g.Edges = append(l.g.Edges, e.Pin.Via)
	}
}

// ref reads a reference from a text node. ok is false when there isn't one.
func (l *loader) ref(from *Node, v *yaml.Node, field string) (Ref, Loc, bool) {
	if v == nil || v.Kind != yaml.ScalarNode || v.Tag != "!!str" || v.Value == "" || strings.ContainsFunc(v.Value, isSpace) {
		return Ref{}, Loc{}, false
	}
	at := Loc{Line: v.Line, Field: field}
	r, err := ParseRef(v.Value)
	if err != nil {
		l.ds.Errorf(from.File.Path, at.Line, "`%s`: %v", field, err)
		return Ref{}, Loc{}, false
	}
	return r, at, true
}

// unwrap drops the path from a file error; the message already names the file.
func unwrap(err error) error {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err
	}
	return err
}
