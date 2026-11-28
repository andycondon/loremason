package graph

// The vocabulary of belief: the support field that holds a belief's support
// links, which way a link points, and the edges that doubt it. The rules that
// read these are for beliefs and live in Go, not types.yaml; see
// docs/context-graph/01-model.md, section 3.2.

// Direction is which way a support link points: toward the belief or against it.
type Direction string

const (
	Supports Direction = "supports"
	Disputes Direction = "disputes"
)

const (
	// supportField is the sub-record field that lists a belief's support links.
	supportField = "support"
	// directionField is the support link field that holds its Direction.
	directionField = "direction"
	// EdgeEvidence is the support link's edge to the evidence it cites.
	EdgeEvidence = "evidence"
	// EdgeUndercuts is evidence's edge to a support link it weakens.
	EdgeUndercuts = "undercuts"
	// EdgeUndermines is evidence's edge to other evidence it weakens.
	EdgeUndermines = "undermines"
)

// HasSupport reports whether the node's type keeps support links.
func (n *Node) HasSupport() bool {
	return !n.IsSubrecord() && n.Type().Subrecords[supportField] != nil
}

// SupportLinks returns the node's support links that point the given way. It
// is empty for a record whose type keeps none.
func (n *Node) SupportLinks(d Direction) []*Node {
	if !n.HasSupport() {
		return nil
	}
	var links []*Node
	for _, link := range n.Children {
		if Direction(link.Fields.Text(directionField)) == d {
			links = append(links, link)
		}
	}
	return links
}
