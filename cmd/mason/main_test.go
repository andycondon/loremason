package main

import (
	"bytes"
	"testing"

	"github.com/andycondon/loremason/internal/testws"
)

// These tests cover the wiring: that each command calls the domain, prints
// what it returns, and exits with the right code. The rules are tested in
// their own packages, and parsing, help and exit codes in package cli.

func mason(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// newWorkspace makes a workspace with records, and points mason at it.
func newWorkspace(t *testing.T, records map[string]string) *testws.Workspace {
	t.Helper()
	useVersion(t, testws.Version)
	w := testws.New(t, records)
	t.Setenv("LOREMASON_WORKSPACE", w.Root)
	return w
}

func useVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

// This is mason's own help, which is part of its documentation.
const help = "mason creates and checks the records of a context graph, keeps the links\n" +
	"between them pinned, and answers questions about them.\n" +
	"\n" +
	"Usage:\n" +
	"  mason <command> [arguments]\n" +
	"\n" +
	"Commands:\n" +
	"  validate     check every record in the workspace\n" +
	"  new          create a record and print its path\n" +
	"  sync         pin links, record fingerprints and regenerate the pages\n" +
	"  revise       open a new revision before changing a record's meaning\n" +
	"  find         list the records whose title or body has all of the words\n" +
	"  show         print a record's text and its links in both directions\n" +
	"  dependents   list the records that rely on a record, directly or through others\n" +
	"  stale        list the records that changed since links were pinned to them, and links to closed records\n" +
	"  reopened     list the commitments resting on a belief that has fallen\n" +
	"  where        show where an initiative stands\n" +
	"\n" +
	"Run `mason <command> --help` for a command's arguments and flags.\n" +
	"\n" +
	"The workspace is the nearest folder, here or above, with a .loremason folder.\n" +
	"Set LOREMASON_WORKSPACE to use another.\n" +
	"\n" +
	"Set LOREMASON_SESSION to name a session; `new` then attaches each finding and\n" +
	"evidence it writes to the session's activity. LOREMASON_INITIATIVE says which\n" +
	"initiative the session is for, and LOREMASON_ACTOR who ran it.\n" +
	"\n" +
	"Flags:\n" +
	"  --version   print the version and exit\n" +
	"  --help      show this help\n"

func TestHelp(t *testing.T) {
	if code, out, _ := mason("--help"); code != 0 || out != help {
		t.Errorf("exit %d:\n%s", code, out)
	}
}

func TestVersion(t *testing.T) {
	if code, out, _ := mason("--version"); code != 0 || out != "mason "+Version+"\n" {
		t.Errorf("exit %d: %q", code, out)
	}
}
