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
	"github.com/andycondon/loremason/internal/record"
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
	want := "---\nid: " + s.ActivityID() + "\ntitle: Session 2026-11-28 (search)\nsession: abc-123\nstarted_at: 2026-11-28T09:30:00Z\npart_of: " + ini + "\nassociated_with:\n  - act_k3m9qx\ncreated: 2026-11-28\n---\n"
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

func TestFromEnvRejectsAnActorThatIsNotAReference(t *testing.T) {
	t.Setenv(EnvSession, "k")
	for _, actor := range []string{"a], status: x", "Andy Condon", "a\nb"} {
		t.Setenv(EnvActor, actor)
		if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), EnvActor) {
			t.Errorf("%q: got %v", actor, err)
		}
	}
	t.Setenv(EnvActor, "andy")
	if s, err := FromEnv(); err != nil || s.Actor != "andy" {
		t.Errorf("an alias: %v, %v", s, err)
	}
}

func TestRenderQuotesTheActor(t *testing.T) {
	// A reference can still hold characters YAML treats as syntax.
	for actor, want := range map[string]string{"a],b": "  - a],b\n", "#x": "  - '#x'\n"} {
		got := (&Session{Key: "k", Actor: actor}).render("actv_000000", index.Initiative{Slug: "search"}, now)
		rec, errs := record.Parse([]byte(got))
		if len(errs) > 0 || !strings.Contains(got, want) || strings.Contains(got, "[") {
			t.Errorf("%s: %v\n%s", actor, errs, got)
			continue
		}
		if v, _ := rec.Header.Get("associated_with"); len(v.Items()) != 1 {
			t.Errorf("%s: %v", actor, v.Plain())
		}
	}
}

func TestFromEnvKey(t *testing.T) {
	t.Setenv(EnvActor, "")
	for _, blank := range []string{"", "   ", " \t "} {
		t.Setenv(EnvSession, blank)
		if s, err := FromEnv(); s != nil || err != nil {
			t.Errorf("%q: got %v, %v", blank, s, err)
		}
	}
	t.Setenv(EnvSession, "  abc  ")
	if s, err := FromEnv(); err != nil || s.Key != "abc" {
		t.Errorf("trimmed: %v, %v", s, err)
	}
	for _, key := range []string{"a\nb", "a\rb"} {
		t.Setenv(EnvSession, key)
		if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), EnvSession) {
			t.Errorf("%q: got %v", key, err)
		}
	}
}

func TestEnsureSaysWhenTheActivitysInitiativeWins(t *testing.T) {
	w := workspace(t)
	w.Write("initiative/ini_d4e5f6.md", "---\nid: ini_d4e5f6\ntitle: Other\nslug: other\nstatus: active\ncreated: 2026-11-22\n---\n\nBody.\n")
	if _, err := (&Session{Key: "k", Initiative: "search"}).Ensure(w.Root, registry.Load(), now); err != nil {
		t.Fatal(err)
	}
	var warn strings.Builder
	rel, err := (&Session{Key: "k", Initiative: "other", Warn: &warn}).Ensure(w.Root, registry.Load(), now)
	if err != nil || !strings.Contains(read(w, rel), "part_of: "+ini) {
		t.Fatalf("%v:\n%s", err, read(w, rel))
	}
	if got := warn.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, "search") || !strings.Contains(got, EnvInitiative+"=other") {
		t.Errorf("warning: %q", got)
	}
	// The same initiative, by an alias, and no initiative: nothing to say.
	warn.Reset()
	for _, name := range []string{"find", "search", ""} {
		if _, err := (&Session{Key: "k", Initiative: name, Warn: &warn}).Ensure(w.Root, registry.Load(), now); err != nil {
			t.Fatal(err)
		}
	}
	if warn.Len() != 0 {
		t.Errorf("warning: %q", warn.String())
	}
}

func TestEnsureWritesAReadableFile(t *testing.T) {
	w := workspace(t)
	rel, err := (&Session{Key: "k", Initiative: "search"}).Ensure(w.Root, registry.Load(), now)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(w.Root, rel)); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("%v, %v", fi, err)
	}
}

func TestStartedAtIsLocal(t *testing.T) {
	w := workspace(t)
	local := time.Date(2026, 11, 28, 23, 30, 0, 0, time.FixedZone("x", -5*3600))
	rel, err := (&Session{Key: "k", Initiative: "search"}).Ensure(w.Root, registry.Load(), local)
	got := read(w, rel)
	if err != nil || !strings.Contains(got, "started_at: 2026-11-28T23:30:00-05:00\n") || !strings.Contains(got, "created: 2026-11-28\n") {
		t.Errorf("%v:\n%s", err, got)
	}
}
