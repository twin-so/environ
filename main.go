package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peter-evans/patience"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

const (
	// SHA256 produces 32-byte hashes
	sha256HashSize = 32
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

// generateArchiveID creates a base64 URL-encoded SHA256 hash of the data
func generateArchiveID(data []byte) string {
	hash := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func push(environ Environ) error {
	// Create ZIP from local files
	zipData, err := getLocalZipData(environ)
	if err != nil {
		return err
	}

	archiveID := generateArchiveID(zipData)

	// Check if already up to date
	if currentRef, err := os.ReadFile(environ.Ref); err == nil && string(currentRef) == archiveID {
		log.Printf("Already up to date: %s", archiveID)
		return nil
	}

	// Upload to remote
	if err := environ.Remote.Write(archiveID, zipData); err != nil {
		return fmt.Errorf("failed to upload archive: %w", err)
	}

	// Update ref file
	if err := os.WriteFile(environ.Ref, []byte(archiveID), 0644); err != nil {
		return fmt.Errorf("failed to update ref file %q: %w", environ.Ref, err)
	}

	log.Printf("Pushed %d files to %s as %s", len(environ.Files), environ.String(), archiveID)
	return nil
}

// isArchiveID checks if a string is a valid archive ID (base64 URL-encoded SHA256)
func isArchiveID(s string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(decoded) == sha256HashSize
}

// readRefFile reads and validates a ref file
func readRefFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read ref file %s: %w", path, err)
	}
	ref := strings.TrimSpace(string(content))
	if ref == "" {
		return "", fmt.Errorf("ref file %s is empty", path)
	}
	return ref, nil
}

// getZipFromSource retrieves ZIP data from either an archive ID or a ref file
// Returns the ZIP data and the resolved archive ID
func getZipFromSource(environ Environ, source string) ([]byte, string, error) {
	var archiveID string

	if isArchiveID(source) {
		archiveID = source
	} else {
		// Treat as ref file
		ref, err := readRefFile(source)
		if err != nil {
			return nil, "", err
		}
		archiveID = ref
	}

	zipData, err := environ.Remote.Get(archiveID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download archive %s: %w", archiveID, err)
	}
	return zipData, archiveID, nil
}

// getLocalZipData creates a ZIP archive from files in the current directory
func getLocalZipData(environ Environ) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for _, file := range environ.Files {
		fileContent, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("tracked file %q not found in current directory", file)
			}
			return nil, fmt.Errorf("failed to read %q: %w", file, err)
		}

		fileWriter, err := zipWriter.Create(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create ZIP entry for %q: %w", file, err)
		}

		if _, err := fileWriter.Write(fileContent); err != nil {
			return nil, fmt.Errorf("failed to write %q to ZIP: %w", file, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize ZIP: %w", err)
	}

	return buf.Bytes(), nil
}

func diffZips(fromZipData, toZipData []byte, fromLabel, toLabel string) error {
	// Create ZIP readers
	fromZipReader, err := zip.NewReader(bytes.NewReader(fromZipData), int64(len(fromZipData)))
	if err != nil {
		return fmt.Errorf("failed to read 'from' ZIP: %w", err)
	}

	toZipReader, err := zip.NewReader(bytes.NewReader(toZipData), int64(len(toZipData)))
	if err != nil {
		return fmt.Errorf("failed to read 'to' ZIP: %w", err)
	}

	// Build file maps
	fromFiles := make(map[string]*zip.File)
	for _, file := range fromZipReader.File {
		fromFiles[file.Name] = file
	}

	toFiles := make(map[string]*zip.File)
	for _, file := range toZipReader.File {
		toFiles[file.Name] = file
	}

	// Check all files
	allFiles := make(map[string]bool)
	for name := range fromFiles {
		allFiles[name] = true
	}
	for name := range toFiles {
		allFiles[name] = true
	}

	// Sort files for consistent output
	var fileList []string
	for file := range allFiles {
		fileList = append(fileList, file)
	}
	sort.Strings(fileList)

	// Diff each file
	for _, fileName := range fileList {
		fromFile, fromExists := fromFiles[fileName]
		toFile, toExists := toFiles[fileName]

		if !fromExists && toExists {
			fmt.Printf("!!! file %s only in %s\n", fileName, toLabel)
			continue
		}
		if fromExists && !toExists {
			fmt.Printf("!!! file %s only in %s\n", fileName, fromLabel)
			continue
		}

		// Both exist, compare contents
		fromReader, err := fromFile.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in %s ZIP: %w", fileName, fromLabel, err)
		}
		fromContent, err := io.ReadAll(fromReader)
		fromReader.Close()
		if err != nil {
			return fmt.Errorf("failed to read file %s in %s ZIP: %w", fileName, fromLabel, err)
		}

		toReader, err := toFile.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in %s ZIP: %w", fileName, toLabel, err)
		}
		toContent, err := io.ReadAll(toReader)
		toReader.Close()
		if err != nil {
			return fmt.Errorf("failed to read file %s in %s ZIP: %w", fileName, toLabel, err)
		}

		if !bytes.Equal(fromContent, toContent) {
			diff := patience.Diff(strings.Split(string(fromContent), "\n"), strings.Split(string(toContent), "\n"))
			unidiff := patience.UnifiedDiffTextWithOptions(
				diff,
				patience.UnifiedDiffOptions{
					Precontext:  1,
					Postcontext: 1,
					SrcHeader:   fmt.Sprintf("%s (%s)", fileName, fromLabel),
					DstHeader:   fmt.Sprintf("%s (%s)", fileName, toLabel),
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

func diffAll(environNames []string, from, to string) error {
	for _, environName := range environNames {
		environ, ok := environs[environName]
		if !ok {
			return envNotFound(environName)
		}

		if err := diffEnviron(environ, from, to); err != nil {
			return fmt.Errorf("failed to diff %s: %w", environName, err)
		}
	}
	return nil
}

// diffEnviron performs diff for a single environment with the given from/to parameters
func diffEnviron(environ Environ, from, to string) error {
	// Resolve from parameter (default to ref file content)
	fromSource := from
	if fromSource == "" {
		ref, err := readRefFile(environ.Ref)
		if err != nil {
			return err
		}
		fromSource = ref
	}

	// Get ZIP data for comparison
	fromZipData, fromID, err := getZipFromSource(environ, fromSource)
	if err != nil {
		return fmt.Errorf("failed to get 'from' source: %w", err)
	}

	var toZipData []byte
	var toLabel string

	if to == "" {
		// Compare with current directory when no -to flag specified
		toZipData, err = getLocalZipData(environ)
		if err != nil {
			return err
		}
		// Generate archive ID for local data to use as label
		toLabel = generateArchiveID(toZipData) + " (local)"
	} else {
		// Compare with another ref
		var toID string
		toZipData, toID, err = getZipFromSource(environ, to)
		if err != nil {
			return fmt.Errorf("failed to get 'to' source: %w", err)
		}
		toLabel = toID
	}

	// Use first 12 chars of archive IDs for brevity in diff output
	fromLabel := fromID
	if len(fromLabel) > 12 {
		fromLabel = fromLabel[:12]
	}
	if len(toLabel) > 12 && !strings.HasSuffix(toLabel, " (local)") {
		toLabel = toLabel[:12]
	}

	return diffZips(fromZipData, toZipData, fromLabel, toLabel)
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
		fmt.Printf("       %s diff [-from ref] [-to ref] [environ ...]\n", os.Args[0])
		fmt.Printf("       (-from defaults to the contents of the ref file; -to defaults to the checked out file)\n")
		printAvailableEnvirons()
		os.Exit(0)
	}

	cmd := os.Args[1]

	// Parse arguments based on command
	var environNames []string
	var from, to string

	if cmd == "diff" {
		// diff command supports optional -from and -to flags
		diffFlags := flag.NewFlagSet("diff", flag.ContinueOnError)
		diffFlags.StringVar(&from, "from", "", "source ref (archive ID or ref file)")
		diffFlags.StringVar(&to, "to", "", "target ref (archive ID or ref file)")

		// Parse flags
		err := diffFlags.Parse(os.Args[2:])
		if err != nil {
			fmt.Printf("Usage: %s diff [-from ref] [-to ref] [environ ...]\n", os.Args[0])
			os.Exit(1)
		}

		// Remaining args after flags are environ names
		environNames = diffFlags.Args()
		if len(environNames) == 0 {
			// Default to all environments
			for name := range environs {
				environNames = append(environNames, name)
			}
		}
	} else {
		// For pull/push commands, all args after command are environ names
		if len(os.Args) > 2 {
			environNames = os.Args[2:]
		} else {
			for name := range environs {
				environNames = append(environNames, name)
			}
		}
	}

	switch cmd {
	case "pull":
		err = pullAll(environNames)
	case "push":
		err = pushAll(environNames)
	case "diff":
		err = diffAll(environNames, from, to)
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
