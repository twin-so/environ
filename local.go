package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
)

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
