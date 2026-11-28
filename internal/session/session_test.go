package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andycondon/loremason/internal/id"
	"github.com/andycondon/loremason/internal/index"
	"github.com/andycondon/loremason/internal/registry"
	"github.com/andycondon/loremason/internal/testws"
)

const ini = "ini_a1b2c3"

var now = time.Date(2026, 11, 28, 9, 30, 0, 0, time.UTC)

func workspace(t *testing.T) *testws.Workspace {
	t.Helper()
	return testws.New(t, map[string]string{
		"initiative/" + ini + ".md": "---\nid: " + ini + "\ntitle: Search\nslug: search\nstatus: active\naliases: [find]\ncreated: 2026-11-22\n---\n\nBody.\n",
	})
}

func TestFromEnv(t *testing.T) {
	t.Setenv(EnvSession, "")
	if s, err := FromEnv(); s != nil || err != nil {
		t.Errorf("unset: got %v, %v", s, err)
	}
	t.Setenv(EnvSession, "abc-123")
	t.Setenv(EnvInitiative, "search")
	t.Setenv(EnvActor, "act_k3m9qx")
	s, err := FromEnv()
	if err != nil || *s != (Session{Key: "abc-123", Initiative: "search", Actor: "act_k3m9qx"}) {
		t.Errorf("got %v, %v", s, err)
	}
}

func TestActivityID(t *testing.T) {
	a := (&Session{Key: "abc-123"}).ActivityID()
	if p, ok := id.Prefix(a); !ok || p != "actv" {
		t.Errorf("got %q", a)
	}
	if a != (&Session{Key: "abc-123", Actor: "x"}).ActivityID() || a == (&Session{Key: "abc-124"}).ActivityID() {
		t.Error("the ID doesn't come from the key alone")
	}
}

func TestEnsureCreatesTheActivity(t *testing.T) {
	w := workspace(t)
	s := &Session{Key: "abc-123", Initiative: "find", Actor: "act_k3m9qx"}
	rel, err := s.Ensure(w.Root, registry.Load(), now)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nid: " + s.ActivityID() + "\ntitle: Session 2026-11-28 (search)\nsession: abc-123\nstarted_at: 2026-11-28T09:30:00Z\npart_of: " + ini + "\nassociated_with: [act_k3m9qx]\ncreated: 2026-11-28\n---\n"
	if rel != "graph/activity/"+s.ActivityID()+".md" || read(w, rel) != want {
		t.Errorf("%s:\n%s", rel, read(w, rel))
	}
}

func TestEnsureWithoutAnActor(t *testing.T) {
	w := workspace(t)
	rel, err := (&Session{Key: "k", Initiative: ini}).Ensure(w.Root, registry.Load(), now)
	if err != nil || strings.Contains(read(w, rel), "associated_with") {
		t.Errorf("%v:\n%s", err, read(w, rel))
	}
}

func TestEnsureTwiceUsesTheSameFile(t *testing.T) {
	w := workspace(t)
	s := &Session{Key: "abc-123", Initiative: "search"}
	rel, err := s.Ensure(w.Root, registry.Load(), now)
	if err != nil {
		t.Fatal(err)
	}
	first := read(w, rel)
	// Later, and without the initiative: the file says where it belongs.
	again, err := (&Session{Key: "abc-123"}).Ensure(w.Root, registry.Load(), now.Add(48*time.Hour))
	if err != nil || again != rel || read(w, rel) != first {
		t.Errorf("%v %s:\n%s", err, again, read(w, rel))
	}
}

func TestEnsureInParallelCreatesOneFile(t *testing.T) {
	w := workspace(t)
	const n = 8
	rels := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rels[i], errs[i] = (&Session{Key: "abc-123", Initiative: "search"}).Ensure(w.Root, registry.Load(), now)
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || rels[i] != rels[0] {
			t.Errorf("%d: %s, %v", i, rels[i], errs[i])
		}
	}
	files, _ := filepath.Glob(filepath.Join(w.Root, "graph", "activity", "*"))
	if len(files) != 1 {
		t.Errorf("files: %v", files)
	}
}

func TestEnsureRefusesAnotherSessionsFile(t *testing.T) {
	s := &Session{Key: "abc-123", Initiative: "search"}
	w := testws.New(t, map[string]string{
		"activity/" + s.ActivityID() + ".md": "---\nid: " + s.ActivityID() + "\ntitle: Other\nsession: someone-else\nstarted_at: 2026-11-28\ncreated: 2026-11-28\n---\n",
	})
	if _, err := s.Ensure(w.Root, registry.Load(), now); err == nil || !strings.Contains(err.Error(), "someone-else") {
		t.Errorf("got %v", err)
	}
}

func TestEnsureUnknownInitiative(t *testing.T) {
	w := workspace(t)
	_, err := (&Session{Key: "k", Initiative: "serch"}).Ensure(w.Root, registry.Load(), now)
	var nf *index.NotFoundError
	if !errors.As(err, &nf) || !strings.Contains(err.Error(), "did you mean `search`?") || !strings.Contains(err.Error(), EnvInitiative) {
		t.Errorf("got %v", err)
	}
	if _, err := os.Stat(w.Path("activity")); err == nil {
		t.Error("an activity was written")
	}
}

func TestEnsureNeedsAnInitiativeForANewActivity(t *testing.T) {
	w := workspace(t)
	_, err := (&Session{Key: "k"}).Ensure(w.Root, registry.Load(), now)
	if !errors.Is(err, ErrNoInitiative) || !strings.Contains(err.Error(), "LOREMASON_INITIATIVE") {
		t.Errorf("got %v", err)
	}
}

// read returns the file at rel, a path from the workspace root.
func read(w *testws.Workspace, rel string) string {
	data, _ := os.ReadFile(filepath.Join(w.Root, rel))
	return string(data)
}
