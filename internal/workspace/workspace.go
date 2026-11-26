// Package workspace finds a loremason workspace and reads its configuration.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Env names the environment variable that points at a workspace explicitly.
const Env = "LOREMASON_WORKSPACE"

const marker = ".loremason"

// Workspace is a folder with a .loremason marker.
type Workspace struct {
	Root   string
	Config Config
}

// Config is .loremason/config.yaml.
type Config struct {
	Version string `yaml:"version"` // the loremason version this workspace uses
}

// Find opens the workspace that override names, or, when override is empty,
// the nearest one at or above dir.
func Find(dir, override string) (*Workspace, error) {
	if override != "" {
		if !isMarked(override) {
			return nil, fmt.Errorf("%s is %s, which has no %s folder", Env, override, marker)
		}
		return open(override)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for d := dir; ; {
		if isMarked(d) {
			return open(d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return nil, fmt.Errorf("no workspace here: no %s folder in %s or any folder above it", marker, dir)
		}
		d = parent
	}
}

func isMarked(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, marker))
	return err == nil && info.IsDir()
}

func open(root string) (*Workspace, error) {
	path := filepath.Join(root, marker, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s is missing; it needs a line such as `version: 0.0.1`", path)
	}
	if err != nil {
		return nil, err
	}
	w := &Workspace{Root: root}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&w.Config); err != nil && err != io.EOF {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if w.Config.Version == "" {
		return nil, fmt.Errorf("%s: `version` is missing", path)
	}
	return w, nil
}
