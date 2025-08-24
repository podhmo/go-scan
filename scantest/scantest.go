package scantest

import (
	"bytes"
	"context"
	"fmt"
	i_fs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/fs"
	"github.com/podhmo/go-scan/scanner"
)

// ActionFunc is a function that performs a check or an action based on scan results.
type ActionFunc func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error

// Result holds the outcome of a Run that has side effects.
type Result struct {
	Outputs  map[string][]byte
	Modified map[string][]byte
}

// RunOption configures a test run.
type RunOption func(*runConfig)

type runConfig struct {
	moduleRoot string
	scanner    *scan.Scanner
}

// WithModuleRoot explicitly sets the module root directory for the test run.
func WithModuleRoot(path string) RunOption {
	return func(c *runConfig) {
		c.moduleRoot = path
	}
}

// WithScanner provides a pre-configured scanner to the Run function.
func WithScanner(s *scan.Scanner) RunOption {
	return func(c *runConfig) {
		c.scanner = s
	}
}

// Run sets up and executes a test scenario.
func Run(t *testing.T, dir string, patterns []string, action ActionFunc, opts ...RunOption) (*Result, error) {
	t.Helper()

	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	initialFiles, err := readFilesInDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scantest: failed to read initial files: %w", err)
	}

	workDir := cfg.moduleRoot
	if workDir == "" {
		foundRoot, err := findModuleRoot(dir)
		if err != nil {
			cwd, err_cwd := os.Getwd()
			if err_cwd != nil {
				return nil, fmt.Errorf("scantest: could not get current working directory: %w", err_cwd)
			}
			foundRoot, err = findModuleRoot(cwd)
			if err != nil {
				return nil, fmt.Errorf("scantest: failed to find go.mod root from temp dir (%s) or cwd (%s)", dir, cwd)
			}
		}
		workDir = foundRoot
	}

	var s *scan.Scanner
	if cfg.scanner != nil {
		s = cfg.scanner
	} else {
		overlay, err := createGoModOverlay(workDir)
		if err != nil {
			return nil, fmt.Errorf("creating go.mod overlay: %w", err)
		}

		fs := newOverlayFS(overlay)

		options := []scan.ScannerOption{
			scan.WithWorkDir(workDir),
			scan.WithGoModuleResolver(),
			scan.WithFS(fs),
			scan.WithOverlay(overlay),
		}

		s, err = scan.New(options...)
		if err != nil {
			return nil, fmt.Errorf("new scanner: %w", err)
		}
	}

	pkgs, err := s.Scan(patterns...)
	if err != nil {
		return nil, fmt.Errorf("initial scan: %w", err)
	}

	ctx := context.Background()
	if err := action(ctx, s, pkgs); err != nil {
		return nil, fmt.Errorf("action: %w", err)
	}

	finalFiles, err := readFilesInDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scantest: failed to read final files: %w", err)
	}

	result := &Result{
		Outputs:  make(map[string][]byte),
		Modified: make(map[string][]byte),
	}
	hasChanges := false

	for path, finalContent := range finalFiles {
		initialContent, ok := initialFiles[path]
		if !ok {
			result.Outputs[path] = finalContent
			hasChanges = true
		} else if !bytes.Equal(initialContent, finalContent) {
			result.Modified[path] = finalContent
			hasChanges = true
		}
	}
	if !hasChanges {
		return nil, nil
	}
	return result, nil
}

// WriteFiles creates a temporary directory and populates it with initial files.
func WriteFiles(t *testing.T, files map[string]string) (string, func()) {
	t.Helper()
	dir := t.TempDir()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}
	return dir, func() {}
}

func readFilesInDir(root string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, d i_fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("failed to get relative path for %s: %w", path, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}
			files[relPath] = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func findModuleRoot(startDir string) (string, error) {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in or above %s", startDir)
		}
		dir = parent
	}
}

func createGoModOverlay(dir string) (scanner.Overlay, error) {
	goModPath := filepath.Join(dir, "go.mod")
	_, err := os.Stat(goModPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat go.mod: %w", err)
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	var modified bool

	inReplaceBlock := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		isReplaceLine := strings.HasPrefix(trimmedLine, "replace")
		if !inReplaceBlock && isReplaceLine && strings.HasSuffix(trimmedLine, "(") {
			inReplaceBlock = true
		} else if inReplaceBlock && trimmedLine == ")" {
			inReplaceBlock = false
		}

		if isReplaceLine || inReplaceBlock {
			parts := strings.Fields(trimmedLine)
			arrowIndex := -1
			for i, p := range parts {
				if p == "=>" {
					arrowIndex = i
					break
				}
			}

			if arrowIndex != -1 && arrowIndex < len(parts)-1 {
				pathPart := parts[len(parts)-1]
				if strings.HasPrefix(pathPart, "./") || strings.HasPrefix(pathPart, "../") {
					absPath, err := filepath.Abs(filepath.Join(dir, pathPart))
					if err != nil {
						return nil, fmt.Errorf("could not make path absolute for %q: %w", pathPart, err)
					}
					parts[len(parts)-1] = absPath
					indent := ""
					if len(line) > len(trimmedLine) {
						indent = line[:strings.Index(line, trimmedLine)]
					}
					line = indent + strings.Join(parts, " ")
					modified = true
				}
			}
		}
		newLines = append(newLines, line)
	}

	if !modified {
		return nil, nil
	}

	overlay := scanner.Overlay{
		goModPath: []byte(strings.Join(newLines, "\n")),
	}
	return overlay, nil
}

type overlayFS struct {
	overlay scanner.Overlay
	realFS  fs.FS
}

func newOverlayFS(overlay scanner.Overlay) fs.FS {
	return &overlayFS{
		overlay: overlay,
		realFS:  fs.NewOSFS(),
	}
}

func (f *overlayFS) ReadFile(name string) ([]byte, error) {
	absName, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	if content, ok := f.overlay[absName]; ok {
		return content, nil
	}
	return f.realFS.ReadFile(name)
}

func (f *overlayFS) Stat(name string) (i_fs.FileInfo, error) {
	absName, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	if content, ok := f.overlay[absName]; ok {
		return &fakeFileInfo{
			name:    filepath.Base(name),
			size:    int64(len(content)),
			modTime: time.Now(),
		}, nil
	}
	return f.realFS.Stat(name)
}

func (f *overlayFS) ReadDir(name string) ([]i_fs.DirEntry, error) {
	realEntries, err := f.realFS.ReadDir(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	merged := make(map[string]i_fs.DirEntry)
	for _, entry := range realEntries {
		merged[entry.Name()] = entry
	}

	absName, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}

	for overlayPath, content := range f.overlay {
		if filepath.Dir(overlayPath) == absName {
			entryName := filepath.Base(overlayPath)
			if _, exists := merged[entryName]; !exists {
				merged[entryName] = i_fs.FileInfoToDirEntry(&fakeFileInfo{
					name:    entryName,
					size:    int64(len(content)),
					modTime: time.Now(),
				})
			}
		}
	}

	result := make([]i_fs.DirEntry, 0, len(merged))
	for _, entry := range merged {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result, nil
}

func (f *overlayFS) WalkDir(root string, fn i_fs.WalkDirFunc) error {
	entries, err := f.ReadDir(root)
	if err != nil {
		return fn(root, nil, err)
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		err := fn(path, entry, nil)
		if err != nil {
			if entry.IsDir() && err == filepath.SkipDir {
				continue
			}
			return err
		}
		if entry.IsDir() {
			err := f.WalkDir(path, fn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type fakeFileInfo struct {
	name    string
	size    int64
	mode    i_fs.FileMode
	modTime time.Time
}

func (fi *fakeFileInfo) Name() string       { return fi.name }
func (fi *fakeFileInfo) Size() int64        { return fi.size }
func (fi *fakeFileInfo) Mode() i_fs.FileMode  { return fi.mode }
func (fi *fakeFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fakeFileInfo) IsDir() bool        { return false }
func (fi *fakeFileInfo) Sys() any           { return nil }
