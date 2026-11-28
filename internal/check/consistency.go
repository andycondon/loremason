package check

import (
	"slices"

	"github.com/andycondon/loremason/internal/graph"
)

// Rules about records that agree with themselves and each other: a belief's
// status against its support, and the constraints an edge declares in
// schema/types.yaml (within, target_status, one_in_force). All are warnings.
// The design is in docs/context-graph/01-model.md, sections 3.2 and 4.

// disputedBelief warns about a belief still in force that has a support
// link that disputes it. The rule is for beliefs and is written here, not
// in types.yaml, because it reads a belief's own fields.
func (c *checker) disputedBelief(f *graph.File) {
	n := f.Node
	if !n.HasSupport() || f.Misplaced() || !n.InForce() {
		return
	}
	for _, link := range n.SupportLinks(graph.Disputes) {
		c.ds.Warnf(f.At(link.Fields.KeyLine(graph.DirectionField)), "the belief is %s, but %s disputes it; a rebut means the belief is incorrect: set its status, or remove the link", statusOf(n), link.ID)
	}
}

// constraints checks the per-link constraints of an edge: within and
// target_status.
func (c *checker) constraints(e *graph.Edge) {
	t := e.Target
	if t == nil || !e.Def.Allows(t.Kind) {
		return // target has already reported it
	}
	if e.Def.Within != "" && t.Parent != nil {
		parent := t.Parent
		if !c.linksTo(e.From, e.Def.Within, parent) {
			c.ds.Warnf(e.At.Pos, "`%s` points at `%s`, a sub-record of %s, but %s has no `%s` link to %s", e.At.Field, e.To, parent.ID, e.From.ID, e.Def.Within, parent.ID)
		}
	}
	// The target's status matters once the source is in force and sealed by
	// its own status, such as an accepted decision; a draft may point ahead.
	if len(e.Def.TargetStatus) > 0 && e.From.InForce() && e.From.SealedByStatus() && !slices.Contains(e.Def.TargetStatus, t.Record().Status()) {
		c.ds.Warnf(e.At.Pos, "`%s` points at %s, which is %s; it should be %s", e.At.Field, t.ID, statusOf(t), orList(e.Def.TargetStatus))
	}
}

// linksTo reports whether a record has a link by an edge to a node.
func (c *checker) linksTo(from *graph.Node, edge string, to *graph.Node) bool {
	for _, o := range from.Links(edge) {
		if o.Target == to {
			return true
		}
	}
	return false
}

// statusOf is a node's record's status for a message, or "of no status".
func statusOf(n *graph.Node) string {
	if s := n.Record().Status(); s != "" {
		return s
	}
	return "of no status"
}

// oneInForce warns when more than one record in force has a link by an edge
// marked one_in_force to the same target. For an edge that settles its
// target, the link must also settle it (an accepted decision answering a
// question, not a proposed one). The second and later links are the ones
// reported.
func (c *checker) oneInForce() {
	type key struct {
		edge string
		to   *graph.Node
	}
	first := map[key]*graph.Edge{}
	for _, e := range c.g.Edges {
		if !e.Def.OneInForce || e.Target == nil || !e.Def.Allows(e.Target.Kind) || !e.From.InForce() {
			continue
		}
		if len(e.Def.Settles) > 0 && !e.Settles() {
			continue
		}
		k := key{e.Name, e.Target}
		earlier := first[k]
		switch {
		case earlier == nil:
			first[k] = e
		case earlier.From != e.From:
			c.ds.Warnf(e.At.Pos, "`%s` points at %s, which %s already has a `%s` link to; only one record in force should (%s is %s, %s is %s)", e.At.Field, e.Target.ID, earlier.From.ID, e.Name, earlier.From.ID, statusOf(earlier.From), e.From.ID, statusOf(e.From))
		}
	}
}
