package main

import (
	"bytes"
	"testing"
)

func mason(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// Parsing, help and exit codes are package cli's, and tested there. This is
// mason's own help, which is part of its documentation.
const help = "mason creates and checks the records of a context graph, and keeps the links\n" +
	"between them pinned.\n" +
	"\n" +
	"Usage:\n" +
	"  mason <command> [arguments]\n" +
	"\n" +
	"Commands:\n" +
	"  validate   check every record in the workspace\n" +
	"  new        create a record and print its path\n" +
	"  sync       pin links and record fingerprints\n" +
	"  revise     open a new revision before changing a record's meaning\n" +
	"\n" +
	"Run `mason <command> --help` for a command's arguments and flags.\n" +
	"\n" +
	"The workspace is the nearest folder, here or above, with a .loremason folder.\n" +
	"Set LOREMASON_WORKSPACE to use another.\n" +
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
