// Package vcs asks git about a workspace's files.
package vcs

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Commit returns the commit that holds a file exactly as it is now, or ""
// when there isn't one: git isn't there, the file isn't tracked, or it has
// changes not yet committed.
func Commit(root, rel string) string {
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	path := filepath.FromSlash(rel)
	if _, err := git("ls-files", "--error-unmatch", "--", path); err != nil {
		return "" // not tracked
	}
	if status, err := git("status", "--porcelain", "--", path); err != nil || status != "" {
		return ""
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return head
}
