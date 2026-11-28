package graph

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/andycondon/loremason/internal/diag"
	"github.com/andycondon/loremason/internal/record"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/suggest"
)

// Load reads every record under root/graph and links them up, in five
// steps: read the files; index the records, their sub-records and
// revisions; index aliases; read the edges; then resolve each edge's target,
// which needs every name indexed first.
//
// The line between Load and the checks: Load reports what keeps a file,
// name or link out of the graph (a folder no type has, a second record with
// the same ID, an alias already taken). Whatever made it into the graph but
// is wrong, such as a link to the wrong type, is for the checks to find.
func Load(root string, reg *registry.Registry) (*Graph, diag.List, error) {
	l := &loader{reg: reg, g: &Graph{nodes: map[string]*Node{}, aliases: map[string]*Node{}}}
	if err := l.readAll(filepath.Join(root, Dir)); err != nil {
		return nil, nil, err
	}
	for _, f := range l.g.Files {
		l.addNodes(f)
	}
	for _, f := range l.g.Files {
		l.addAliases(f)
	}
	for _, f := range l.g.Files {
		l.addEdges(f.Node)
		for _, c := range f.Node.Children {
			l.addEdges(c)
		}
	}
	for _, e := range l.g.Edges {
		e.From.Outgoing = append(e.From.Outgoing, e)
		if e.Target = l.g.Resolve(e.To); e.Target != nil {
			e.Target.Incoming = append(e.Target.Incoming, e)
		}
	}
	return l.g, l.ds, nil
}

type loader struct {
	reg *registry.Registry
	g   *Graph
	ds  diag.List
}

// readAll reads every record file under dir: graph/<type-folder>/<id>.md.
// Anything else there is reported and skipped.
func (l *loader) readAll(dir string) error {
	dir, err := filepath.EvalSymlinks(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a new workspace has no records yet
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if p == dir {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		rel = path.Join(Dir, filepath.ToSlash(rel))
		start := diag.Pos{Path: rel, Line: 1}
		inTypeFolder := path.Dir(rel) != Dir
		switch {
		case err != nil:
			l.ds.Errorf(start, "can't read: %v", unwrap(err))
		case strings.HasPrefix(d.Name(), "."):
			if d.IsDir() {
				return fs.SkipDir
			}
		case d.IsDir():
			if !l.isTypeFolder(rel, inTypeFolder) {
				return fs.SkipDir
			}
		case !inTypeFolder:
			l.ds.Errorf(start, "records are kept in a folder named after their type, such as graph/belief/")
		case !isRegularFile(p, d) || path.Ext(rel) != ".md":
			l.ds.Errorf(start, "only record files (<id>.md) belong here")
		default:
			l.read(p, rel)
		}
		return nil
	})
}

// isTypeFolder reports whether a folder is one records belong in, and says
// why not when it isn't.
func (l *loader) isTypeFolder(rel string, nested bool) bool {
	name, start := path.Base(rel), diag.Pos{Path: rel, Line: 1}
	switch {
	case nested:
		l.ds.Errorf(start, "records aren't kept in folders inside a type's folder")
	case l.reg.ByFolder(name) == nil:
		l.ds.Errorf(start, "no record type has the folder `%s`%s", name, suggest.Hint(suggest.Near(name, l.reg.Folders())))
	default:
		return true
	}
	return false
}

// isRegularFile reports whether a path is a file, following a symlink.
func isRegularFile(p string, d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink == 0 {
		return d.Type().IsRegular()
	}
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// read adds one file, and parses it if it can.
func (l *loader) read(p, rel string) {
	folder := l.reg.ByFolder(path.Base(path.Dir(rel)))
	f := &File{Path: rel, Folder: folder, Type: folder}
	rid := idFromPath(rel)
	if t := l.reg.TypeOfID(rid); t != nil {
		f.Type = t
	}
	f.Node = &Node{ID: rid, Kind: &f.Type.Kind, File: f}
	l.g.Files = append(l.g.Files, f)

	data, err := os.ReadFile(p)
	if err != nil {
		l.ds.Errorf(f.At(1), "can't read: %v", unwrap(err))
		return
	}
	f.Sum = sha256.Sum256(data)
	rec, errs := record.Parse(data)
	f.Record, f.rejected = rec, map[int]bool{}
	for _, e := range errs {
		l.ds.Errorf(f.At(e.Line), "%s", e.Msg)
		f.rejected[e.Line] = true
	}
	if rec != nil {
		f.Node.Fields = rec.Header
	}
}

// addNodes indexes a file's record and its sub-records.
func (l *loader) addNodes(f *File) {
	n := f.Node
	if first := l.g.nodes[n.ID]; first != nil {
		l.ds.Errorf(f.At(1), "`%s` is also the ID of %s", n.ID, first.File.Path)
	} else {
		l.g.nodes[n.ID] = n
	}
	readRevisions(n)
	for _, field := range slices.Sorted(maps.Keys(f.Type.Subrecords)) {
		for _, entry := range n.Fields.Entries(field) {
			local := entry.Text("id")
			switch {
			case local == "":
				// the schema check says what's missing
			case n.child(local) != nil:
				l.ds.Errorf(f.At(entry.KeyLine("id")), "two entries have the id `%s`", local)
			default:
				c := &Node{ID: subrecordID(n.ID, local), Kind: f.Type.Subrecords[field], File: f, Fields: entry, Parent: n, Local: local}
				n.Children = append(n.Children, c)
				if l.g.nodes[c.ID] == nil { // a duplicate record's sub-records still resolve
					l.g.nodes[c.ID] = c
				}
			}
		}
	}
}

// addAliases indexes a record's other names.
func (l *loader) addAliases(f *File) {
	aliases, _ := f.Node.Fields.Get("aliases")
	for _, item := range aliases.Items() {
		alias, ok := item.Text()
		if !ok || alias == "" {
			continue
		}
		if other := l.g.nodes[alias]; other != nil {
			l.ds.Errorf(f.At(item.Line()), "the alias `%s` is the ID of a record in %s", alias, other.File.Path)
		} else if other := l.g.aliases[alias]; other != nil && other != f.Node {
			l.ds.Errorf(f.At(item.Line()), "the alias `%s` is already used by %s", alias, other.File.Path)
		} else {
			l.g.aliases[alias] = f.Node
		}
	}
}

// addEdges reads the links in a node's edge fields. Each field holds a link
// or a list of them, and each link is a reference, or {to: …} with perhaps a
// pin. Values of the wrong shape are skipped; the schema check reports them.
func (l *loader) addEdges(from *Node) {
	defs := from.Kind.Edges
	for _, name := range slices.Sorted(maps.Keys(defs)) {
		value, _ := from.Fields.Get(name)
		for _, item := range value.Items() {
			target, p := item, (*Pin)(nil)
			m, isMap := item.Map()
			if isMap {
				target, _ = m.Get("to")
				p = readPin(m)
			}
			to, at, ok := ref(from.File, target)
			if !ok {
				continue
			}
			edge := &Edge{From: from, Name: name, Def: defs[name], To: to, Pin: p, At: at, Item: item}
			l.g.Edges = append(l.g.Edges, edge)
			// A pin's via is a link of its own, to the evidence for the last
			// re-check, so it's checked and indexed like any other.
			if via, ok := m.Get("via"); isMap && p != nil && ok {
				if to, at, ok := ref(from.File, via); ok {
					edge.Via = &Edge{From: from, Name: "via", Def: ViaEdge, To: to, At: at, Item: via}
					l.g.Edges = append(l.g.Edges, edge.Via)
				}
			}
		}
	}
}

// ViaEdge is the edge from a pin to the evidence for its last re-check; it
// says which records can be a via's target.
var ViaEdge = registry.Edge{To: []string{"evidence"}}

// ref reads a reference. ok is false when the value isn't one; the schema
// check reports why.
func ref(f *File, v record.Value) (Ref, Loc, bool) {
	s, _ := v.Text()
	r, err := ParseRef(s)
	if err != nil {
		return Ref{}, Loc{}, false
	}
	return r, Loc{Pos: f.At(v.Line()), Field: v.Path}, true
}

// unwrap drops the path from a file error; the message already names the file.
func unwrap(err error) error {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err
	}
	return err
}
