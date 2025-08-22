package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"go/ast"
	"go/token"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"gopkg.in/yaml.v3"
)

var (
	update = flag.Bool("update", false, "update golden files")
	debug  = flag.Bool("debug", false, "enable debug logging")
)

func newTestLogger(w io.Writer) *slog.Logger {
	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func TestDocgen(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	// Setup: Run the analysis once and reuse the result for all sub-tests.
	logger := newTestLogger(os.Stderr)
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	analyzer, err := NewAnalyzer(s, logger) // no options needed for the default test
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath, "NewServeMux"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Define test cases for each format
	testCases := []struct {
		format      string
		goldenFile  string
		marshalFunc func(io.Writer, *openapi.OpenAPI) error
	}{
		{
			format:     "yaml",
			goldenFile: "golden.yaml",
			marshalFunc: func(w io.Writer, spec *openapi.OpenAPI) error {
				enc := yaml.NewEncoder(w)
				return enc.Encode(spec)
			},
		},
		{
			format:     "json",
			goldenFile: "golden.json",
			marshalFunc: func(w io.Writer, spec *openapi.OpenAPI) error {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(spec)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			var got bytes.Buffer
			if err := tc.marshalFunc(&got, apiSpec); err != nil {
				t.Fatalf("failed to marshal OpenAPI spec to %s: %v", tc.format, err)
			}

			goldenPath := filepath.Join("testdata", tc.goldenFile)
			if *update {
				if err := os.WriteFile(goldenPath, got.Bytes(), 0644); err != nil {
					t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
				}
				t.Logf("golden file updated: %s", goldenPath)
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				// If the file doesn't exist and we're not in update mode,
				// fail with a helpful message.
				if os.IsNotExist(err) && !*update {
					t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenPath)
				}
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}

			if diff := cmp.Diff(string(want), got.String()); diff != "" {
				t.Errorf("OpenAPI spec mismatch for %s (-want +got):\n%s", tc.format, diff)
			}
		})
	}
}

func TestDocgen_withCustomPatterns(t *testing.T) {
	// Note: This test runs docgen on a package that is a separate Go module
	// located in testdata/custom-patterns.
	moduleDir := "testdata/custom-patterns"
	patternsFile := filepath.Join(moduleDir, "patterns.go")
	goldenFile := filepath.Join("testdata", "custom-patterns.golden.json")
	apiPath := "custom-patterns"

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		logger := newTestLogger(io.Discard)

		customPatterns, err := LoadPatternsFromConfig(patternsFile, logger, s)
		if err != nil {
			return fmt.Errorf("failed to load custom patterns: %w", err)
		}

		var opts []any
		for _, p := range customPatterns {
			opts = append(opts, p)
		}
		analyzer, err := NewAnalyzer(s, logger, opts...)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}

		if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
			return fmt.Errorf("failed to analyze package: %+v", err)
		}
		apiSpec := analyzer.OpenAPI

		var got bytes.Buffer
		enc := json.NewEncoder(&got)
		enc.SetIndent("", "  ")
		if err := enc.Encode(apiSpec); err != nil {
			return fmt.Errorf("failed to marshal OpenAPI spec to json: %w", err)
		}

		if *update {
			if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write golden file %s: %w", goldenFile, err)
			}
			t.Logf("golden file updated: %s", goldenFile)
			return nil
		}
		want, err := os.ReadFile(goldenFile)
		if err != nil {
			return fmt.Errorf("failed to read golden file %s: %w", goldenFile, err)
		}
		if diff := cmp.Diff(string(want), got.String()); diff != "" {
			return fmt.Errorf("OpenAPI spec mismatch for custom patterns (-want +got):\n%s", diff)
		}
		return nil
	}

	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

func TestDocgen_fullParameters(t *testing.T) {
	moduleDir := "testdata/full-parameters"
	patternsFile := filepath.Join(moduleDir, "patterns.go")
	goldenFile := filepath.Join("testdata", "full-parameters.golden.json")
	apiPath := "full-parameters" // module name

	type recordingTracer struct {
		visitedNodePositions map[token.Pos]bool
	}
	tracer := &recordingTracer{visitedNodePositions: make(map[token.Pos]bool)}
	visit := func(node ast.Node) {
		if node != nil {
			tracer.visitedNodePositions[node.Pos()] = true
		}
	}

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		logger := newTestLogger(io.Discard)

		customPatterns, err := LoadPatternsFromConfig(patternsFile, logger, s)
		if err != nil {
			return fmt.Errorf("failed to load custom patterns: %w", err)
		}

		var opts []any
		for _, p := range customPatterns {
			opts = append(opts, p)
		}
		opts = append(opts, WithTracer(symgo.TracerFunc(visit)))
		analyzer, err := NewAnalyzer(s, logger, opts...)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}

		if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
			return fmt.Errorf("failed to analyze package: %+v", err)
		}
		apiSpec := analyzer.OpenAPI

		// Assert that the tracer visited all nodes in the target handler.
		scannedPkg, err := s.ScanPackage(ctx, moduleDir)
		if err != nil {
			return fmt.Errorf("could not re-scan package to find handler AST: %w", err)
		}
		var handlerFunc *goscan.FunctionInfo
		for _, f := range scannedPkg.Functions {
			if f.Name == "GetResource" {
				handlerFunc = f
				break
			}
		}
		if handlerFunc == nil {
			return fmt.Errorf("could not find handler function 'GetResource' to verify tracer")
		}
		var targetNode ast.Node
		ast.Inspect(handlerFunc.AstDecl, func(n ast.Node) bool {
			if targetNode != nil {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.Ident); ok && sel.Name == "GetPathValue" {
					targetNode = call
					return false
				}
			}
			return true
		})
		if targetNode == nil {
			return fmt.Errorf("could not find the target GetPathValue call in the AST")
		}
		if !tracer.visitedNodePositions[targetNode.Pos()] {
			return fmt.Errorf("tracer did not visit the target GetPathValue call expression")
		}

		var got bytes.Buffer
		enc := json.NewEncoder(&got)
		enc.SetIndent("", "  ")
		if err := enc.Encode(apiSpec); err != nil {
			return fmt.Errorf("failed to marshal OpenAPI spec to json: %w", err)
		}

		if *update {
			if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write golden file %s: %w", goldenFile, err)
			}
			t.Logf("golden file updated: %s", goldenFile)
			return nil
		}
		want, err := os.ReadFile(goldenFile)
		if err != nil {
			return fmt.Errorf("failed to read golden file %s: %w", goldenFile, err)
		}
		if diff := cmp.Diff(string(want), got.String()); diff != "" {
			return fmt.Errorf("OpenAPI spec mismatch for full parameters (-want +got):\n%s", diff)
		}
		return nil
	}

	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

func TestDocgen_newFeatures(t *testing.T) {
	moduleDir := "testdata/new-features"
	patternsFile := filepath.Join(moduleDir, "patterns.go")
	goldenFile := filepath.Join(moduleDir, "new-features.golden.json")
	apiPath := "new-features/main"

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		logger := newTestLogger(io.Discard)

		customPatterns, err := LoadPatternsFromConfig(patternsFile, logger, s)
		if err != nil {
			return fmt.Errorf("failed to load custom patterns: %w", err)
		}

		var opts []any
		for _, p := range customPatterns {
			opts = append(opts, p)
		}
		analyzer, err := NewAnalyzer(s, logger, opts...)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}

		if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
			return fmt.Errorf("failed to analyze package: %+v", err)
		}
		apiSpec := analyzer.OpenAPI

		var got bytes.Buffer
		enc := json.NewEncoder(&got)
		enc.SetIndent("", "  ")
		if err := enc.Encode(apiSpec); err != nil {
			return fmt.Errorf("failed to marshal OpenAPI spec to json: %w", err)
		}

		if *update {
			if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write golden file %s: %w", goldenFile, err)
			}
			t.Logf("golden file updated: %s", goldenFile)
			return nil
		}

		want, err := os.ReadFile(goldenFile)
		if err != nil {
			return fmt.Errorf("failed to read golden file %s: %w", goldenFile, err)
		}

		var wantNormalized, gotNormalized any
		if err := json.Unmarshal(want, &wantNormalized); err != nil {
			return fmt.Errorf("failed to unmarshal want JSON: %w", err)
		}
		if err := json.Unmarshal(got.Bytes(), &gotNormalized); err != nil {
			return fmt.Errorf("failed to unmarshal got JSON: %w", err)
		}

		if diff := cmp.Diff(wantNormalized, gotNormalized); diff != "" {
			gotJSON, _ := json.MarshalIndent(gotNormalized, "", "  ")
			return fmt.Errorf("OpenAPI spec mismatch for new features (-want +got):\n%s\n\nGot:\n%s", diff, gotJSON)
		}
		return nil
	}

	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

func TestDocgen_refAndRename(t *testing.T) {
	// This test verifies that:
	// 1. Reused structs result in a single schema in #/components/schemas with $ref used everywhere else.
	// 2. Structs with the same name in different packages are treated as distinct schemas.
	// 3. Handlers with the same name in different packages get unique operation IDs.
	const apiPath = "ref-and-rename/api" // The module name + package dir to analyze
	moduleDir := "testdata/ref-and-rename"
	goldenFile := filepath.Join(moduleDir, "api.golden.json") // Golden file inside the test module

	logger := newTestLogger(io.Discard)

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithWorkDir(moduleDir), // Point scanner to the module directory
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	analyzer, err := NewAnalyzer(s, logger)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "NewServeMux"); err != nil {
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
