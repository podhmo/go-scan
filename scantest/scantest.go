package scantest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

// memoryFileWriter is an in-memory implementation of scan.FileWriter for testing.
type memoryFileWriter struct {
	mu      sync.Mutex
	Outputs map[string][]byte
}

// WriteFile captures the output in memory instead of writing to disk.
func (w *memoryFileWriter) WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Outputs == nil {
		w.Outputs = make(map[string][]byte)
	}
	// In tests, we often care about the relative path from the test's root dir.
	// The path passed here will be absolute. We may need to strip the temp dir prefix.
	w.Outputs[filepath.Base(path)] = data // Storing with basename for simplicity.
	return nil
}

// ActionFunc is a function that performs a check or an action based on scan results.
// For actions with side effects, it should use go-scan's top-level functions
// (e.g., goscan.WriteFile) to allow the test harness to capture the results.
type ActionFunc func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error

// Result holds the outcome of a Run that has side effects.
type Result struct {
	// Outputs contains the content of files written by go-scan's helper functions.
	// The key is the file path, and the value is the content.
	Outputs map[string][]byte
}

// Run sets up and executes a test scenario.
// It returns a Result object if the action had side effects captured by the harness.
func Run(t *testing.T, dir string, patterns []string, action ActionFunc) (*Result, error) {
	t.Helper()

	// Automatically handle go.mod replace directives with relative paths.
	overlay, err := createGoModOverlay(dir)
	if err != nil {
		return nil, fmt.Errorf("creating go.mod overlay: %w", err)
	}

	options := []scan.ScannerOption{scan.WithWorkDir(dir)}
	if overlay != nil {
		options = append(options, scan.WithOverlay(overlay))
	}

	s, err := scan.New(options...)
	if err != nil {
		return nil, fmt.Errorf("new scanner: %w", err)
	}

	pkgs, err := s.Scan(patterns...)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	writer := &memoryFileWriter{}
	ctx := context.WithValue(context.Background(), scan.FileWriterKey, writer)

	if err := action(ctx, s, pkgs); err != nil {
		return nil, fmt.Errorf("action: %w", err)
	}

	if len(writer.Outputs) > 0 {
		return &Result{Outputs: writer.Outputs}, nil
	}
	return nil, nil
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
	return dir, func() { /* t.TempDir handles cleanup */ }
}

// createGoModOverlay reads the go.mod file in the given directory,
// and if it contains `replace` directives with relative file paths,
// it creates an in-memory overlay with those paths converted to absolute paths.
// This is necessary because the `go-scan` tool resolves paths relative
// to the module root, and in tests, the temp directory is not the CWD.
func createGoModOverlay(dir string) (scanner.Overlay, error) {
	goModPath := filepath.Join(dir, "go.mod")
	_, err := os.Stat(goModPath)
	if os.IsNotExist(err) {
		return nil, nil // No go.mod, no overlay needed.
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
			// A valid replace line looks like: "replace module/path => ../local/path"
			// or inside a block: "module/path => ../local/path"
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
					// This is a relative path, resolve it against `dir`
					absPath, err := filepath.Abs(filepath.Join(dir, pathPart))
					if err != nil {
						return nil, fmt.Errorf("could not make path absolute for %q: %w", pathPart, err)
					}
					// Reconstruct the line with the absolute path
					parts[len(parts)-1] = absPath
					// Get the original line's indentation
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
		return nil, nil // No changes, no overlay needed.
	}

	overlay := scanner.Overlay{
		"go.mod": []byte(strings.Join(newLines, "\n")),
	}
	return overlay, nil
}
