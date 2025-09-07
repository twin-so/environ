package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
)

// fs provides a filesystem-based remote backend, storing blobs under a root path
// and optional prefix, with write-once semantics (do not overwrite existing keys).
func fsfunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	root := ""
	prefix := ""
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &root, "prefix?", &prefix); err != nil {
		return nil, err
	}
	if strings.HasPrefix(root, "~/") {
		root = filepath.Join(os.Getenv("HOME"), root[2:])
	}
	if prefix == "" {
		prefix = "environ"
	}
	// Ensure base directory exists
	if err := os.MkdirAll(filepath.Join(root, prefix), 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", filepath.Join(root, prefix), err)
	}
	return FS{
		root:   root,
		prefix: prefix,
	}, nil
}

type FS struct {
	root   string
	prefix string
}

func (f FS) keyPath(key string) string {
	return filepath.Join(f.root, f.prefix, key)
}

func (f FS) Get(key string) ([]byte, error) {
	return os.ReadFile(f.keyPath(key))
}

// Write implements write-once semantics: creating the file only if absent.
// If the file already exists, it is treated as success (idempotent).
func (f FS) Write(key string, value []byte) error {
	// Ensure directory exists (prefix already created, but key may include subdirs)
	if err := os.MkdirAll(filepath.Dir(f.keyPath(key)), 0700); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}
	// Use O_EXCL to avoid overwriting existing content
	fp, err := os.OpenFile(f.keyPath(key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			// already exists: treat as success
			return nil
		}
		return err
	}
	defer fp.Close()
	if _, err := fp.Write(value); err != nil {
		return err
	}
	return nil
}

func (f FS) String() string {
	return fmt.Sprintf("fs(%s, %s)", f.root, f.prefix)
}

func (f FS) Type() string {
	return "FS"
}

func (f FS) Freeze() {}

func (f FS) Truth() starlark.Bool {
	return starlark.Bool(true)
}

func (f FS) Hash() (uint32, error) {
	return starlark.String(f.String()).Hash()
}
