package graph

import (
	"path"
	"strings"

	"github.com/andycondon/loremason/internal/registry"
)

// Dir is the folder in a workspace that holds every record.
const Dir = "graph"

// RecordPath is where a record is kept, relative to the workspace:
// graph/<type's folder>/<id>.md.
func RecordPath(t *registry.Type, id string) string {
	return path.Join(Dir, t.Folder(), id+".md")
}

// idFromPath is the ID a record file's name gives.
func idFromPath(rel string) string { return strings.TrimSuffix(path.Base(rel), ".md") }
