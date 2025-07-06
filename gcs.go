package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"go.starlark.net/starlark"
)

func gcsfunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	bucket := ""
	prefix := ""
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "bucket", &bucket, "prefix?", &prefix); err != nil {
		return nil, err
	}
	if prefix == "" {
		prefix = "environ"
	}
	client, err := storage.NewClient(context.Background())
	if err != nil {
		return nil, err
	}
	return GCS{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

type GCS struct {
	client *storage.Client
	bucket string
	prefix string
}

func (g GCS) Get(key string) ([]byte, error) {
	reader, err := g.client.Bucket(g.bucket).Object(g.prefix + "/" + key).NewReader(context.Background())
	if err != nil {
		return []byte{}, err
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}

func realWriteError(err error) bool {
	return err != nil && !strings.Contains(err.Error(), "conditionNotMet")
}

func (g GCS) Write(key string, value []byte) error {
	writer := g.client.Bucket(g.bucket).Object(g.prefix + "/" + key).If(storage.Conditions{DoesNotExist: true}).NewWriter(context.Background())
	if _, err := writer.Write([]byte(value)); realWriteError(err) {
		writer.Close()
		return err
	}
	if err := writer.Close(); realWriteError(err) {
		return err
	}
	return nil
}

func (g GCS) String() string {
	return fmt.Sprintf("gcs(%s, %s)", g.bucket, g.prefix)
}

func (g GCS) Type() string {
	return "GCS"
}

func (g GCS) Freeze() {
}

func (g GCS) Truth() starlark.Bool {
	return starlark.Bool(true)
}

func (g GCS) Hash() (uint32, error) {
	return starlark.String(g.String()).Hash()
}
