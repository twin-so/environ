package main

import (
	"fmt"

	"go.starlark.net/starlark"
)

func cache(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var of, by Remote
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "of", &of, "by", &by); err != nil {
		return nil, err
	}
	return Cache{
		Of: of,
		By: by,
	}, nil
}

type Cache struct {
	By Remote
	Of Remote
}

func (c Cache) Get(key string) ([]byte, error) {
	if cached, err := c.By.Get(key); err == nil {
		return cached, nil
	}
	content, err := c.Of.Get(key)
	if err != nil {
		return nil, err
	}
	if err := c.By.Write(key, content); err != nil {
		return nil, err
	}
	return content, nil
}

func (c Cache) Write(key string, value []byte) error {
	if err := c.Of.Write(key, value); err != nil {
		return err
	}
	return c.By.Write(key, value)
}

func (c Cache) String() string {
	return fmt.Sprintf("Cache(%s, %s)", c.By, c.Of)
}

func (c Cache) Type() string {
	return "Cache"
}

func (c Cache) Freeze() {
}

func (c Cache) Truth() starlark.Bool {
	return starlark.Bool(true)
}

func (c Cache) Hash() (uint32, error) {
	return starlark.String(c.String()).Hash()
}
