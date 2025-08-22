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

	loadedPatterns, err := LoadPatternsFromConfig("patterns.go", logger, s)
	if err != nil {
		t.Fatalf("LoadPatternsFromConfig failed: %+v", err)
	}

	expectedKeys := map[string]bool{
		"key-from-fn/foo.(*Foo).Bar": true, // from nil, pointer literal, and new
		"key-from-fn/foo.Foo.Qux":    true, // from value
		"key-from-fn/foo.Baz":        true, // from standalone function
	}

	if len(loadedPatterns) != 5 {
		t.Fatalf("expected 5 patterns, got %d", len(loadedPatterns))
	}

	foundKeys := make(map[string]bool)
	for _, p := range loadedPatterns {
		foundKeys[p.Key] = true
	}

	if diff := cmp.Diff(expectedKeys, foundKeys); diff != "" {
		// The diff can be tricky because multiple patterns map to the same key.
		// Let's do a more explicit check.
		t.Logf("key mismatch (-want +got):\n%s", diff)

		// Manual check for clarity
		for k := range expectedKeys {
			if !foundKeys[k] {
				t.Errorf("expected key %q was not found", k)
			}
		}
		for k := range foundKeys {
			if !expectedKeys[k] {
				t.Errorf("found unexpected key %q", k)
			}
		}
	}
}
