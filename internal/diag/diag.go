// Package diag holds the errors and warnings a check finds, each at a place
// in a file.
package diag

import (
	"cmp"
	"fmt"
	"slices"
)

// Diagnostic is one thing wrong with a file.
type Diagnostic struct {
	Path    string // relative to the workspace, with forward slashes
	Line    int
	Warning bool
	Msg     string
}

func (d Diagnostic) String() string {
	level := "error"
	if d.Warning {
		level = "warning"
	}
	return fmt.Sprintf("%s:%d: %s: %s", d.Path, d.Line, level, d.Msg)
}

// List collects diagnostics.
type List []Diagnostic

// Errorf adds an error.
func (l *List) Errorf(path string, line int, format string, args ...any) {
	*l = append(*l, Diagnostic{Path: path, Line: line, Msg: fmt.Sprintf(format, args...)})
}

// Warnf adds a warning.
func (l *List) Warnf(path string, line int, format string, args ...any) {
	*l = append(*l, Diagnostic{Path: path, Line: line, Warning: true, Msg: fmt.Sprintf(format, args...)})
}

// Sort orders diagnostics by file, then line, then message, so output is the
// same on every run.
func (l List) Sort() {
	slices.SortStableFunc(l, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Msg, b.Msg))
	})
}

// Count returns the number of errors and warnings.
func (l List) Count() (errs, warnings int) {
	for _, d := range l {
		if d.Warning {
			warnings++
		} else {
			errs++
		}
	}
	return
}
