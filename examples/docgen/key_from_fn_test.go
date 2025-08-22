package main

import (
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
)

func TestKeyFromFn(t *testing.T) {
	moduleDir := "testdata/key-from-fn"

	// Setup: Change directory to the testdata so the module can be resolved.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}
	if err := os.Chdir(moduleDir); err != nil {
		t.Fatalf("could not change directory to %s: %v", moduleDir, err)
	}
	defer os.Chdir(wd)

	// Create a scanner configured to find the new module.
	logger := newTestLogger(io.Discard)
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Load the patterns from the actual file.
	// The unmarshaling will be into the real `patterns.PatternConfig`, not the stub.
	// This works as long as the field names and types are compatible.
	loadedPatterns, err := LoadPatternsFromConfig("patterns.go", logger, s)
	if err != nil {
		t.Fatalf("LoadPatternsFromConfig failed: %+v", err)
	}

	if len(loadedPatterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(loadedPatterns))
	}

	expectedKeys := map[string]bool{
		"key-from-fn/foo.(*Foo).Bar": true,
		"key-from-fn/foo.Baz":        true,
	}

	foundKeys := make(map[string]bool)
	for _, p := range loadedPatterns {
		foundKeys[p.Key] = true
	}

	if diff := cmp.Diff(expectedKeys, foundKeys); diff != "" {
		t.Errorf("key mismatch (-want +got):\n%s", diff)
	}
}
