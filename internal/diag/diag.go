// Package diag holds the errors and warnings a check finds, each at a place
// in a file.
package diag

import (
	"cmp"
	"fmt"
	"slices"
)

// Pos is a place in a file.
type Pos struct {
	Path string // relative to the workspace, with forward slashes
	Line int
}

// Diagnostic is one thing wrong with a file.
type Diagnostic struct {
	Pos
	Warning bool
	Fixable bool // an error that `mason sync` repairs, so it doesn't keep sync from the file
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
func (l *List) Errorf(at Pos, format string, args ...any) {
	*l = append(*l, Diagnostic{Pos: at, Msg: fmt.Sprintf(format, args...)})
}

// Fixablef adds an error that `mason sync` repairs.
func (l *List) Fixablef(at Pos, format string, args ...any) {
	*l = append(*l, Diagnostic{Pos: at, Fixable: true, Msg: fmt.Sprintf(format, args...)})
}

// Warnf adds a warning.
func (l *List) Warnf(at Pos, format string, args ...any) {
	*l = append(*l, Diagnostic{Pos: at, Warning: true, Msg: fmt.Sprintf(format, args...)})
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
