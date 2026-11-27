package testws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/check"
	"github.com/andycondon/loremason/internal/graph"
	"github.com/andycondon/loremason/internal/registry"
)

func generated(t *testing.T, n int, seed int64) (string, *registry.Registry) {
	t.Helper()
	reg := registry.Load()
	dir := t.TempDir()
	if err := Generate(dir, n, seed); err != nil {
		t.Fatal(err)
	}
	return dir, reg
}

// ---- Generate ----

func TestGenerateIsValid(t *testing.T) {
	dir, reg := generated(t, 400, 1)
	res, err := check.Run(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range res.Diagnostics {
		t.Logf("%v", d)
	}
	if errs, _ := res.Diagnostics.Count(); errs != 0 {
		t.Errorf("%d errors", errs)
	}
	if res.Records < 300 || res.Records > 500 {
		t.Errorf("records = %d, want about 400", res.Records)
	}
}

func TestGenerateIsReproducible(t *testing.T) {
	a, _ := generated(t, 200, 7)
	b, _ := generated(t, 200, 7)
	c, _ := generated(t, 200, 8)
	if !sameFiles(t, a, b) {
		t.Error("same seed gave different files")
	}
	if sameFiles(t, a, c) {
		t.Error("different seeds gave the same files")
	}
}

func TestGenerateHasStalePinsAndReopeningEdges(t *testing.T) {
	dir, reg := generated(t, 400, 3)
	g, _, err := graph.Load(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	var stale, reopens int
	for _, e := range g.Edges {
		if e.Stale() {
			stale++
		}
		if e.Reopens() {
			reopens++
		}
	}
	if stale == 0 || reopens == 0 {
		t.Errorf("stale = %d, reopens = %d, want some of each", stale, reopens)
	}
}

func TestGenerateIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	start := time.Now()
	if err := Generate(t.TempDir(), 3000, 1); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("3000 records took %v", d)
	}
}

func sameFiles(t *testing.T, a, b string) bool {
	t.Helper()
	read := func(root string) map[string]string {
		m := map[string]string{}
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				data, _ := os.ReadFile(p)
				rel, _ := filepath.Rel(root, p)
				m[rel] = string(data)
			}
			return nil
		})
		return m
	}
	x, y := read(a), read(b)
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if y[k] != v {
			return false
		}
	}
	return true
}
