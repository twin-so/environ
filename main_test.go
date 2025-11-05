package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("failed to close reader: %v", err)
	}

	return string(output)
}

func zipData(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	var filenames []string
	for name := range entries {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)

	for _, name := range filenames {
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(entries[name])); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	return buf.Bytes()
}

func TestDiffZipsPrintsContentForAddedFile(t *testing.T) {
	fromZip := zipData(t, map[string]string{})
	toZip := zipData(t, map[string]string{
		"jaiminho/browse_agent/.env.sandbox": "FOO=bar\nBAZ=qux\n",
	})

	var changed bool
	var err error
	output := captureOutput(t, func() {
		changed, err = diffZips(fromZip, toZip, "QlgiIViuR", "rXtcTkVBF")
	})
	if err != nil {
		t.Fatalf("diffZips returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected diffZips to report changes for added file")
	}

	if !strings.Contains(output, "!!! file jaiminho/browse_agent/.env.sandbox only in rXtcTkVBF") {
		t.Fatalf("expected output to mention added file, got:\n%s", output)
	}
	if !strings.Contains(output, "+FOO=bar") || !strings.Contains(output, "+BAZ=qux") {
		t.Fatalf("expected added file content to be printed as + lines, got:\n%s", output)
	}
	if !strings.Contains(output, "--- jaiminho/browse_agent/.env.sandbox (QlgiIViuR)") {
		t.Fatalf("expected diff header for added file, got:\n%s", output)
	}
}

func TestDiffZipsPrintsContentForDeletedFile(t *testing.T) {
	fromZip := zipData(t, map[string]string{
		"jaiminho/browse_agent/.env.prod": "SECRET=value\n",
	})
	toZip := zipData(t, map[string]string{})

	var changed bool
	var err error
	output := captureOutput(t, func() {
		changed, err = diffZips(fromZip, toZip, "QlgiIViuR", "rXtcTkVBF")
	})
	if err != nil {
		t.Fatalf("diffZips returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected diffZips to report changes for deleted file")
	}

	if !strings.Contains(output, "!!! file jaiminho/browse_agent/.env.prod only in QlgiIViuR") {
		t.Fatalf("expected output to mention deleted file, got:\n%s", output)
	}
	if !strings.Contains(output, "-SECRET=value") {
		t.Fatalf("expected deleted file content to be printed as - lines, got:\n%s", output)
	}
	if !strings.Contains(output, "--- jaiminho/browse_agent/.env.prod (QlgiIViuR)") {
		t.Fatalf("expected diff header for deleted file, got:\n%s", output)
	}
}
