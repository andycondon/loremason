package graph

import (
	"testing"

	"github.com/andycondon/loremason/internal/registry"
)

func TestRecordPath(t *testing.T) {
	doc, _ := registry.Load().Find("design_doc")
	if got := RecordPath(doc, "dd_c5r1tw"); got != "graph/design-doc/dd_c5r1tw.md" {
		t.Errorf("RecordPath = %q", got)
	}
}

func TestIDFromPath(t *testing.T) {
	if got := idFromPath("graph/belief/bel_7k3m9q.md"); got != "bel_7k3m9q" {
		t.Errorf("idFromPath = %q", got)
	}
}
