package pinning

import (
	"strings"
	"testing"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

const today = "2026-11-25"

// checked checks the workspace, as every writing tool does first.
func checked(t *testing.T, w *testws.Workspace) *check.Result {
	t.Helper()
	res, err := check.Run(w.Root, registry.Load())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// problems is what validate would report, one diagnostic per line.
func problems(t *testing.T, w *testws.Workspace) string {
	t.Helper()
	var lines []string
	for _, d := range checked(t, w).Diagnostics {
		lines = append(lines, d.String())
	}
	return strings.Join(lines, "\n")
}

// sync does what `mason sync` does: check, plan, apply. It returns the plan.
func sync(t *testing.T, w *testws.Workspace) *Plan {
	t.Helper()
	res := checked(t, w)
	p := Sync(res.Graph, res.Broken(), today)
	if err := Apply(w.Root, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// tryRevise plans `mason revise` for a record, outside git, and applies it.
func tryRevise(t *testing.T, w *testws.Workspace, id string, breaking bool) (*Plan, error) {
	t.Helper()
	res := checked(t, w)
	n, err := res.Graph.Lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Revise(n, breaking, res.Broken(), today, "")
	if err != nil {
		return nil, err
	}
	if err := Apply(w.Root, p); err != nil {
		t.Fatal(err)
	}
	return p, nil
}

func revise(t *testing.T, w *testws.Workspace, id string, breaking bool) *Plan {
	t.Helper()
	p, err := tryRevise(t, w, id, breaking)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
