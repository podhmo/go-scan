package scantest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	scan "github.com/podhmo/go-scan"
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
	s, err := scan.New(scan.WithWorkDir(dir))
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
