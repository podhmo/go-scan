package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

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
	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed, indicating a failure in loading patterns: %+v", err)
	}
}
