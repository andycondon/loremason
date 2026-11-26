package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := run("--version")
	if code != 0 || out != "mason "+Version+"\n" {
		t.Fatalf("got code %d, output %q", code, out)
	}
}

func TestNoArgumentsPrintsUsage(t *testing.T) {
	code, out, _ := run()
	if code != 0 || !strings.Contains(out, "Usage:\n  mason") {
		t.Fatalf("got code %d, output %q", code, out)
	}
}

func TestHelp(t *testing.T) {
	code, _, errOut := run("--help")
	if code != 0 || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("got code %d, stderr %q", code, errOut)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, errOut := run("--nope")
	if code != 2 || !strings.Contains(errOut, "nope") {
		t.Fatalf("got code %d, stderr %q", code, errOut)
	}
}
