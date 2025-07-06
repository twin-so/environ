package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
)

func local(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	path := ""
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(os.Getenv("HOME"), path[2:])
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return Local{
		path: path,
	}, nil
}

type Local struct {
	path string
}

func (l Local) Get(key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(l.path, key))
}

func (l Local) Write(key string, value []byte) error {
	return os.WriteFile(filepath.Join(l.path, key), value, 0644)
}

func (l Local) String() string {
	return fmt.Sprintf("local(%s)", l.path)
}

func (l Local) Type() string {
	return "Local"
}

func (l Local) Freeze() {
}

func (l Local) Truth() starlark.Bool {
	return starlark.Bool(true)
}

func (l Local) Hash() (uint32, error) {
	return starlark.String(l.String()).Hash()
}
