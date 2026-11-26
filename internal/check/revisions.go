package check

import (
	"fmt"

	"github.com/andycondon/loremason/internal/graph"
)

// Rules about revisions and pins. The design is in
// docs/context-graph/03-implementation.md, sections 4 and 5.

// revisionNumbers checks that a record's revisions run 1, 2, 3 with no gaps.
func (c *checker) revisionNumbers(f *graph.File) {
	for i, r := range f.Node.Revisions {
		if r.Rev != i+1 {
			c.ds.Errorf(r.At.Pos, "revisions are numbered 1, 2, 3 and so on; this one is %d where %d belongs", r.Rev, i+1)
			return
		}
	}
}

// sealed checks that a revision something is built on hasn't changed its
// meaning. Changing it means a new revision, made before the edit, so the
// old wording can be kept.
func (c *checker) sealed(f *graph.File) {
	n := f.Node
	cur := n.Current()
	if cur == nil || f.Record == nil || f.Misplaced() || !n.Sealed(cur.Rev) {
		return
	}
	if cur.FP.IsZero() {
		c.ds.Errorf(cur.At.Pos, "revision %d is pinned but its fingerprint isn't recorded; run `mason sync`", cur.Rev)
		return
	}
	if cur.FP == n.Fingerprint() {
		return
	}
	pinnedBy := n.PinnedBy(cur.Rev)
	c.ds.Errorf(f.At(meaningLine(f)), "the meaning changed, but revision %d is pinned by %s: undo the change, run `mason revise %s --breaking` (or `--non-breaking`), then make it again", cur.Rev, andList(pinnedBy), n.ID)
}

// meaningLine is where a record's meaning starts: its first meaning field,
// or line 1 when its only meaning is the body.
func meaningLine(f *graph.File) int {
	for _, name := range f.Type.Meaning {
		if name != "body" {
			return f.Node.Fields.KeyLine(name)
		}
	}
	return 1
}

// pinning checks the revision a link names, in a pin or as @rN: the record
// must keep revisions, and the revision must exist. A sub-record's revision
// is its record's, as in bel_7k3m9q/s1@r2. Only an edge that its type pins
// may carry a pin.
func (c *checker) pinning(e *graph.Edge) {
	if e.Target == nil {
		return // target has already reported it
	}
	rec, at := e.Target.Record(), e.Item.Path
	rev, how := e.To.Rev, fmt.Sprintf("`%s`", e.To)
	if e.Pin != nil {
		rev, how = e.Pin.Rev, "the pin"
	}
	switch {
	case rev == 0:
	case !rec.Revisable():
		c.ds.Errorf(e.At.Pos, "`%s`: %s names revision %d, but %s has no revisions; write just `%s`", at, how, rev, a(rec.Kind.Name), e.To.Name)
		return
	case rec.Revision(rev) == nil:
		c.ds.Errorf(e.At.Pos, "`%s`: %s names revision %d of %s, which has %s", at, how, rev, rec.ID, revisionCount(rec))
		return
	}
	if e.Pin != nil && !e.Def.ShouldPin(e.Target.Kind) {
		c.ds.Errorf(e.At.Pos, "`%s` isn't pinned, since a change in %s doesn't call for a re-check here; write just `%s`", at, e.Target.ID, e.To.Name)
	}
}

func revisionCount(n *graph.Node) string {
	switch len(n.Revisions) {
	case 0:
		return "no revisions"
	case 1:
		return "only revision 1"
	}
	return fmt.Sprintf("revisions 1 to %d", len(n.Revisions))
}
