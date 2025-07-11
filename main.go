package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/peter-evans/patience"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type EnvNotFound struct {
	name string
}

func (e EnvNotFound) Error() string {
	return fmt.Sprintf("environment %s not found", e.name)
}

func envNotFound(name string) EnvNotFound {
	return EnvNotFound{name: name}
}

type Environ struct {
	Remote
	Files []string
	Ref   string
}

type Remote interface {
	Get(key string) ([]byte, error)
	Write(key string, value []byte) error

	String() string
	Type() string
	Freeze()
	Truth() starlark.Bool
	Hash() (uint32, error)
}

var (
	opts = syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}
	environs = map[string]Environ{}
)

func environ(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, ref string
	var remote Remote
	var files *starlark.List

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "remote", &remote, "files", &files, "ref", &ref); err != nil {
		return nil, err
	}
	var fileList []string = make([]string, files.Len())
	for i := 0; i < files.Len(); i++ {
		fileList[i] = files.Index(i).(starlark.String).GoString()
	}

	if _, ok := environs[name]; ok {
		return starlark.None, fmt.Errorf("environ %s declared multiple times", name)
	}

	environs[name] = Environ{
		Remote: remote,
		Files:  fileList,
		Ref:    ref,
	}
	return starlark.None, nil
}

func fileHasChanged(filename string, newContent []byte) (bool, error) {
	existingContent, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return !bytes.Equal(existingContent, newContent), nil
}

func pull(environ Environ) error {
	refContent, err := os.ReadFile(environ.Ref)
	if err != nil {
		return fmt.Errorf("failed to read ref file %s: %w", environ.Ref, err)
	}
	if len(refContent) == 0 {
		return fmt.Errorf("ref file %s is empty", environ.Ref)
	}

	ref := strings.TrimSpace(string(refContent))

	zipData, err := environ.Remote.Get(ref)
	if err != nil {
		return fmt.Errorf("failed to download ZIP %s: %w", ref, err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader([]byte(zipData)), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to read ZIP: %w", err)
	}

	expectedFiles := make(map[string]bool)
	for _, file := range environ.Files {
		expectedFiles[file] = true
	}

	zipFiles := make(map[string]bool)
	for _, file := range zipReader.File {
		zipFiles[file.Name] = true
	}

	missingFiles := []string{}
	for _, file := range environ.Files {
		if !zipFiles[file] {
			missingFiles = append(missingFiles, file)
		}
	}
	if len(missingFiles) > 0 {
		return fmt.Errorf("missing files in ZIP: %v", missingFiles)
	}

	extraneousFiles := []string{}
	for _, file := range zipReader.File {
		if !expectedFiles[file.Name] {
			extraneousFiles = append(extraneousFiles, file.Name)
		}
	}
	if len(extraneousFiles) > 0 {
		return fmt.Errorf("extraneous files in ZIP: %v", extraneousFiles)
	}

	changedFiles := 0
	for _, file := range zipReader.File {
		dir := filepath.Dir(file.Name)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in ZIP: %w", file.Name, err)
		}

		fileContent, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to read file %s from ZIP: %w", file.Name, err)
		}

		hasChanged, err := fileHasChanged(file.Name, fileContent)
		if err != nil {
			return fmt.Errorf("failed to check if file %s has changed: %w", file.Name, err)
		}

		if hasChanged {
			localFile, err := os.Create(file.Name)
			if err != nil {
				return fmt.Errorf("failed to create local file %s: %w", file.Name, err)
			}

			_, err = localFile.Write(fileContent)
			localFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", file.Name, err)
			}
			changedFiles++
		}
	}

	if changedFiles > 0 {
		log.Printf("Changed %d/%d files from %s", changedFiles, len(environ.Files), ref)
	}
	return nil
}

func push(environ Environ) error {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for _, file := range environ.Files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", file)
		}

		zipFile, err := zipWriter.Create(file)
		if err != nil {
			return fmt.Errorf("failed to create ZIP entry for %s: %w", file, err)
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", file, err)
		}

		_, err = zipFile.Write(content)
		if err != nil {
			return fmt.Errorf("failed to write file %s to ZIP: %w", file, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close ZIP: %w", err)
	}

	bytes := buf.Bytes()
	hash := sha256.Sum256(bytes)
	key := base64.RawURLEncoding.EncodeToString(hash[:])

	if currentRef, err := os.ReadFile(environ.Ref); err == nil && string(currentRef) == key {
		return nil
	}

	if err := environ.Remote.Write(key, bytes); err != nil {
		return fmt.Errorf("failed to upload ZIP: %w", err)
	}

	err := os.WriteFile(environ.Ref, []byte(key), 0644)
	if err != nil {
		return fmt.Errorf("failed to update ref file: %w", err)
	}

	log.Printf("Pushed %d files to %s as %s", len(environ.Files), environ.String(), key)
	return nil
}

func diff(environ Environ) error {
	refContent, err := os.ReadFile(environ.Ref)
	if err != nil {
		return fmt.Errorf("failed to read ref file %s: %w", environ.Ref, err)
	}
	ref := strings.TrimSpace(string(refContent))

	zipData, err := environ.Remote.Get(ref)
	if err != nil {
		return fmt.Errorf("failed to download ZIP %s: %w", ref, err)
	}

	localFiles := make(map[string]bool)
	for _, file := range environ.Files {
		localFiles[file] = true
	}

	for _, file := range environ.Files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", file)
		}
	}

	zipReader, err := zip.NewReader(bytes.NewReader([]byte(zipData)), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to read ZIP: %w", err)
	}

	zipFiles := make(map[string]bool)
	for _, file := range zipReader.File {
		zipFiles[file.Name] = true
		if !localFiles[file.Name] {
			fmt.Printf("!!! file %s in remote but not working directory\n", file.Name)
		}
	}

	for _, file := range environ.Files {
		if !zipFiles[file] {
			fmt.Printf("!!! file %s in working directory but not remote\n", file)
			continue
		}
		zipFile, err := zipReader.Open(file)
		if err != nil {
			return fmt.Errorf("failed to open file %s in ZIP: %w", file, err)
		}
		zipFileContent, err := io.ReadAll(zipFile)
		if err != nil {
			return fmt.Errorf("failed to read file %s in ZIP: %w", file, err)
		}
		zipFile.Close()
		localFileContent, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file %s in working directory: %w", file, err)
		}
		if !bytes.Equal(zipFileContent, localFileContent) {
			diff := patience.Diff(strings.Split(string(zipFileContent), "\n"), strings.Split(string(localFileContent), "\n"))
			unidiff := patience.UnifiedDiffTextWithOptions(
				diff,
				patience.UnifiedDiffOptions{
					Precontext:  1,
					Postcontext: 1,
					SrcHeader:   fmt.Sprintf("%s (remote)", file),
					DstHeader:   fmt.Sprintf("%s (local)", file),
				},
			)
			fmt.Print(unidiff)
		}
	}

	return nil
}

func pullAll(environNames []string) error {
	for _, environName := range environNames {
		environ, ok := environs[environName]
		if !ok {
			return envNotFound(environName)
		}
		if err := pull(environ); err != nil {
			return fmt.Errorf("failed to pull %s: %w", environName, err)
		}
	}
	return nil
}

func pushAll(environNames []string) error {
	for _, environName := range environNames {
		environ, ok := environs[environName]
		if !ok {
			return envNotFound(environName)
		}
		if err := push(environ); err != nil {
			return fmt.Errorf("failed to push %s: %w", environName, err)
		}
	}
	return nil
}

func diffAll(environNames []string) error {
	for _, environName := range environNames {
		environ, ok := environs[environName]
		if !ok {
			return envNotFound(environName)
		}
		if err := diff(environ); err != nil {
			return fmt.Errorf("failed to diff %s: %w", environName, err)
		}
	}
	return nil
}

func printAvailableEnvirons() {
	environNames := make([]string, 0, len(environs))
	for name := range environs {
		environNames = append(environNames, name)
	}
	fmt.Printf("Available environs: %s\n", strings.Join(environNames, ", "))
}

func main() {
	// Find the parent directory of `environ.star` in the ancestor directories of the current working directory
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "environ.star")); err == nil {
			break
		}
		dir = filepath.Dir(dir)
		if dir == "/" {
			log.Fatal("environ.star not found")
		}
	}
	os.Chdir(dir)

	thread := starlark.Thread{
		Name: "environ",
	}

	globals := starlark.StringDict{
		"gcs":     starlark.NewBuiltin("gcs", gcsfunc),
		"s3":      starlark.NewBuiltin("s3", s3func),
		"local":   starlark.NewBuiltin("local", local),
		"cache":   starlark.NewBuiltin("cache", cache),
		"environ": starlark.NewBuiltin("environ", environ),
	}

	_, err = starlark.ExecFileOptions(&opts, &thread, "environ.star", nil, globals)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s pull|push|diff [environ ...]\n", os.Args[0])
		printAvailableEnvirons()
		os.Exit(0)
	}

	environNames := []string{}
	if len(os.Args) > 2 {
		environNames = os.Args[2:]
	} else {
		for name := range environs {
			environNames = append(environNames, name)
		}
	}

	cmd := os.Args[1]
	switch cmd {
	case "pull":
		err = pullAll(environNames)
	case "push":
		err = pushAll(environNames)
	case "diff":
		err = diffAll(environNames)
	default:
		log.Printf("%s is not a valid command", cmd)
		os.Exit(1)
	}
	if err != nil {
		log.Printf("Error: %s", err)
		var envNotFound EnvNotFound
		if errors.As(err, &envNotFound) {
			printAvailableEnvirons()
		}
		os.Exit(1)
	}
}
