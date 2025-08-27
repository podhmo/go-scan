package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestDocgen_WithFnPatterns(t *testing.T) {
	// This test reproduces the scenario from docs/trouble-docgen-minigo-import.md.
	// It verifies that docgen can load a minigo configuration script (`patterns.go`)
	// from a nested Go module (`testdata/integration/fn-patterns`), and that this
	// script can successfully import other packages.
	// This relies on the `replace` directives in the nested module's go.mod
	// being correctly handled by the go-scan locator.

	// The test now uses fixed files in `testdata/integration` instead of
	// creating them on the fly.

	// The module we want to operate within is the nested one.
	moduleDir := filepath.Join("testdata", "integration", "fn-patterns")
	patternsFile := filepath.Join(moduleDir, "patterns.go")

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// The scanner `s` is pre-configured by scantest.Run to have its
		// WorkDir set to `moduleDir`, with a go.mod overlay that resolves
		// the relative `replace` paths.
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		// Call the docgen loader function, which will use the provided scanner.
		_, err := LoadPatternsFromConfig(patternsFile, logger, s)
		return err // If err is nil, the import was successful.
	}

	// Use scantest.Run to drive the test.
	// It's crucial to set the module root to `moduleDir` so the scanner
	// finds the correct `go.mod` and resolves the `replace` directive.
	// The first argument to Run is also the module dir, as that's the
	// context for this test run.
	if _, err := scantest.Run(t, context.Background(), moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed, indicating a failure in loading patterns: %+v", err)
	}
}

func TestDocgen_WithFnPatterns_FullAnalysis(t *testing.T) {
	// This test verifies that a pattern defined using the type-safe `Fn` field
	// works correctly in a full docgen analysis, producing the expected OpenAPI output.
	const apiPath = "my-test-module"
	moduleDir := filepath.Join("testdata", "integration", "fn-patterns-full")
	patternsFile := filepath.Join(moduleDir, "patterns.go")
	goldenFile := filepath.Join("testdata", "integration", "fn-patterns-full.golden.json")

	logger := newTestLogger(os.Stderr)

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithWorkDir(moduleDir), // Point scanner to the module directory
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Load custom patterns from the config file.
	customPatterns, err := LoadPatternsFromConfig(patternsFile, logger, s)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}

	// Create an analyzer with the custom patterns.
	var opts []any
	for _, p := range customPatterns {
		opts = append(opts, p)
	}
	analyzer, err := NewAnalyzer(s, logger, nil, nil, opts...)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	if err := analyzer.Analyze(context.Background(), apiPath, "main"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Marshal the result to JSON.
	var got bytes.Buffer
	enc := json.NewEncoder(&got)
	enc.SetIndent("", "  ")
	if err := enc.Encode(apiSpec); err != nil {
		t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
	}

	// Compare with the golden file.
	if *update {
		if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenFile, err)
		}
		t.Logf("golden file updated: %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenFile)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
	}

	// Normalize JSON to avoid cosmetic diffs
	var wantNormalized, gotNormalized any
	if err := json.Unmarshal(want, &wantNormalized); err != nil {
		t.Fatalf("failed to unmarshal want JSON: %v", err)
	}
	if err := json.Unmarshal(got.Bytes(), &gotNormalized); err != nil {
		t.Fatalf("failed to unmarshal got JSON: %v", err)
	}

	if diff := cmp.Diff(wantNormalized, gotNormalized); diff != "" {
		gotJSON, _ := json.MarshalIndent(gotNormalized, "", "  ")
		t.Errorf("OpenAPI spec mismatch (-want +got):\n%s\n\nGot:\n%s", diff, gotJSON)
	}
}
