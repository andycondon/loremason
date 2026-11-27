package main

import (
	"os"
	"strings"
	"testing"
)

func TestValidatePrintsProblemsAndASummary(t *testing.T) {
	useVersion(t, "test")
	const fixture = "../../testdata/check/wrong-target-type"
	// validate opens the index, which writes to the workspace, so it runs on
	// a copy.
	copy := t.TempDir()
	if err := os.CopyFS(copy, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOREMASON_WORKSPACE", copy)
	want, err := os.ReadFile(fixture + "/want.txt")
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := mason("validate")
	if code != 1 || errOut != "" || out != string(want)+"4 records: 4 errors, 0 warnings\n" {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
}

func TestValidateCleanWorkspace(t *testing.T) {
	newWorkspace(t, nil)
	if code, out, errOut := mason("validate"); code != 0 || errOut != "" || out != "0 records: 0 errors, 0 warnings\n" {
		t.Errorf("exit %d, stderr %q, stdout %q", code, errOut, out)
	}
}

func TestVersionMismatchWarns(t *testing.T) {
	newWorkspace(t, nil)
	useVersion(t, "other")
	if code, _, errOut := mason("validate"); code != 0 || errOut != "mason: warning: this is mason other, but the workspace uses test\n" {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestNoWorkspace(t *testing.T) {
	t.Setenv("LOREMASON_WORKSPACE", "")
	t.Chdir(t.TempDir())
	if code, _, errOut := mason("validate"); code != 1 || !strings.Contains(errOut, "no workspace here") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}
