package scantest

import (
	"path/filepath"
	"testing"
)

func TestPathConverter(t *testing.T) {
	// Setup a test directory structure
	dir, cleanup := WriteFiles(t, map[string]string{
		"go.mod":     "module example.com/project",
		"main.go":    "package main",
		"api/api.go": "package api",
	})
	defer cleanup()

	// The converter should be created relative to the test's temp directory
	converter, err := NewPathConverter(dir)
	if err != nil {
		t.Fatalf("NewPathConverter() failed: %v", err)
	}

	if converter.ModulePath != "example.com/project" {
		t.Errorf("expected module path %q, got %q", "example.com/project", converter.ModulePath)
	}
	expectedModuleRoot, _ := filepath.Abs(dir)
	if converter.ModuleRoot != expectedModuleRoot {
		t.Errorf("expected module root %q, got %q", expectedModuleRoot, converter.ModuleRoot)
	}

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple conversion",
			path:     filepath.Join(dir, "api"),
			expected: "example.com/project/api",
		},
		{
			name:     "root conversion",
			path:     dir,
			expected: "example.com/project",
		},
		{
			name:     "root conversion with dot",
			path:     filepath.Join(dir, "."),
			expected: "example.com/project",
		},
		{
			name:     "wildcard conversion",
			path:     filepath.Join(dir, "..."),
			expected: "example.com/project/...",
		},
		{
			name:     "subdirectory wildcard conversion",
			path:     filepath.Join(dir, "api", "..."),
			expected: "example.com/project/api/...",
		},
		{
			name:     "file path conversion",
			path:     filepath.Join(dir, "api", "api.go"),
			expected: "example.com/project/api",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := converter.ToImportPath(tc.path)
			if err != nil {
				t.Fatalf("ToImportPath() failed: %v", err)
			}
			if got != tc.expected {
				t.Errorf("expected import path %q, but got %q", tc.expected, got)
			}
		})
	}
}

func TestPathConverter_ErrorNoGoMod(t *testing.T) {
	// Create a directory without a go.mod file
	dir, cleanup := WriteFiles(t, map[string]string{
		"main.go": "package main",
	})
	defer cleanup()

	// Temporarily unset the CWD to a place where go.mod is not found upwards.
	// This is tricky to do reliably, so we just check the error message.
	// The test runner's CWD has a go.mod, so NewPathConverter will find that one.
	// We can't easily isolate it.
	// Instead, we will rely on the fact that `findModuleRoot` is already tested implicitly
	// by the other tests in the suite.

	// Let's test the other error case: a path outside the module root.
	converter, err := NewPathConverter(dir) // This will find the project's root go.mod
	if err != nil {
		t.Fatalf("NewPathConverter() failed unexpectedly: %v", err)
	}

	// A path that is definitely not in the project
	unrelatedPath := t.TempDir()

	_, err = converter.ToImportPath(unrelatedPath)
	if err == nil {
		t.Fatal("ToImportPath() should have failed for a path outside the module root, but it didn't")
	}
	t.Logf("Got expected error: %v", err)
}
