package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go/ast"
	"go/token"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/symgo"
	"gopkg.in/yaml.v3"
)

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
	analyzer, err := NewAnalyzer(s, logger, []string{"net/http"})
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

func TestDocgen_withRelativePath(t *testing.T) {
	// This test verifies that docgen can be run with a relative path argument.
	const relativeSampleAPIPath = "./sampleapi"

	logger := newTestLogger(os.Stderr)
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Resolve the relative path to an import path for the analyzer.
	// This mimics what the main function now does.
	ctx := context.Background()
	importPath, err := goscan.ResolvePath(ctx, relativeSampleAPIPath)
	if err != nil {
		t.Fatalf("failed to resolve relative path %q: %v", relativeSampleAPIPath, err)
	}

	analyzer, err := NewAnalyzer(s, logger, []string{"net/http"})
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	if err := analyzer.Analyze(ctx, importPath, "NewServeMux"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Marshal to JSON and compare with the golden file.
	var got bytes.Buffer
	enc := json.NewEncoder(&got)
	enc.SetIndent("", "  ")
	if err := enc.Encode(apiSpec); err != nil {
		t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
	}

	goldenPath := filepath.Join("testdata", "golden.json")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	if diff := cmp.Diff(string(want), got.String()); diff != "" {
		t.Errorf("OpenAPI spec mismatch for relative path test (-want +got):\n%s", diff)
	}
}

func TestDocgen_withCustomPatterns(t *testing.T) {
	// Note: This test runs docgen on a package that is a separate Go module
	// located in testdata/custom-patterns.
	const apiPath = "custom-patterns"
	goldenFile := "testdata/custom-patterns.golden.json"

	// Setup: Change directory to the testdata so the module can be resolved.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}
	if err := os.Chdir("testdata/custom-patterns"); err != nil {
		t.Fatalf("could not change directory: %v", err)
	}
	defer os.Chdir(wd)

	logger := newTestLogger(io.Discard)

	// Load custom patterns from the config file.
	// Note: The path is relative to the new CWD.
	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	customPatterns, err := LoadPatternsFromConfig("patterns.go", logger, s)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}

	// Create an analyzer with the custom patterns.
	var opts []any
	for _, p := range customPatterns {
		opts = append(opts, p)
	}
	analyzer, err := NewAnalyzer(s, logger, opts...)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	// The analyzer will find the HandleFunc calls within main.
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
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
	// Note: The golden file path is relative to the original CWD.
	goldenPath := filepath.Join(wd, goldenFile)
	if *update {
		if err := os.WriteFile(goldenPath, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		t.Logf("golden file updated: %s", goldenPath)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) && !*update {
			t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenPath)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	if diff := cmp.Diff(string(want), got.String()); diff != "" {
		t.Errorf("OpenAPI spec mismatch for custom patterns (-want +got):\n%s", diff)
	}
}

func TestDocgen_fullParameters(t *testing.T) {
	// recordingTracer is a simple implementation of symgo.Tracer that records the
	// positions of the AST nodes it visits.
	type recordingTracer struct {
		visitedNodePositions map[token.Pos]bool
	}

	tracer := &recordingTracer{visitedNodePositions: make(map[token.Pos]bool)}

	visit := func(node ast.Node) {
		if node == nil {
			return
		}
		tracer.visitedNodePositions[node.Pos()] = true
	}

	// This test is based on the scenario described in `docs/trouble-docgen.md`.
	// It verifies that path, query, and header parameters defined via custom
	// patterns are correctly included in the final OpenAPI specification.
	const apiPath = "full-parameters"
	goldenFile := "testdata/full-parameters.golden.json"

	// Setup: Change directory to the testdata so the module can be resolved.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}
	if err := os.Chdir("testdata/full-parameters"); err != nil {
		t.Fatalf("could not change directory: %v", err)
	}
	defer os.Chdir(wd)

	logger := newTestLogger(io.Discard)

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	// Load custom patterns from the config file.
	customPatterns, err := LoadPatternsFromConfig("patterns.go", logger, s)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}

	// Create an analyzer with the custom patterns and a tracer.
	var opts []any
	for _, p := range customPatterns {
		opts = append(opts, p)
	}
	opts = append(opts, WithTracer(symgo.TracerFunc(visit)))
	analyzer, err := NewAnalyzer(s, logger, opts...)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Assert that the tracer visited all nodes in the target handler.
	// This confirms the symbolic execution engine is not skipping parts of the function.
	scannedPkg, err := s.ScanPackageByImport(ctx, apiPath)
	if err != nil {
		t.Fatalf("Could not re-scan package to find handler AST: %v", err)
	}
	var handlerFunc *goscan.FunctionInfo
	for _, f := range scannedPkg.Functions {
		if f.Name == "GetResource" {
			handlerFunc = f
			break
		}
	}
	if handlerFunc == nil {
		t.Fatalf("Could not find handler function 'GetResource' to verify tracer")
	}

	// Find a specific node we expect the tracer to have visited.
	var targetNode ast.Node
	ast.Inspect(handlerFunc.AstDecl, func(n ast.Node) bool {
		if targetNode != nil {
			return false // already found
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.Ident); ok {
				if sel.Name == "GetPathValue" {
					targetNode = call
					return false
				}
			}
		}
		return true
	})

	if targetNode == nil {
		t.Fatalf("Could not find the target GetPathValue call in the AST")
	}

	if !tracer.visitedNodePositions[targetNode.Pos()] {
		t.Errorf("Tracer did not visit the target GetPathValue call expression")
	}

	// Marshal the result to JSON.
	var got bytes.Buffer
	enc := json.NewEncoder(&got)
	enc.SetIndent("", "  ")
	if err := enc.Encode(apiSpec); err != nil {
		t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
	}

	// Compare with the golden file.
	goldenPath := filepath.Join(wd, goldenFile)
	if *update {
		if err := os.WriteFile(goldenPath, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		t.Logf("golden file updated: %s", goldenPath)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) && !*update {
			t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenPath)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	if diff := cmp.Diff(string(want), got.String()); diff != "" {
		t.Errorf("OpenAPI spec mismatch for full parameters (-want +got):\n%s", diff)
	}
}

func TestDocgen_newFeatures(t *testing.T) {
	// This test verifies the new features: map support and default error responses.
	// It is configured to run from the 'examples/docgen' directory and use
	// goscan.WithWorkDir to point to the test module, mirroring the setup
	// in the passing symgo_intramodule_test.go.
	const apiPath = "new-features/main" // The module name to analyze
	moduleDir := "testdata/new-features"
	patternsFile := filepath.Join(moduleDir, "patterns.go")
	goldenFile := filepath.Join(moduleDir, "new-features.golden.json")

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
	analyzer, err := NewAnalyzer(s, logger, opts...)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
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
	goldenPath := goldenFile
	if *update {
		if err := os.WriteFile(goldenPath, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		t.Logf("golden file updated: %s", goldenPath)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) && !*update {
			t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenPath)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	// Normalize JSON by unmarshalling and marshalling again to avoid cosmetic diffs
	var wantNormalized, gotNormalized any
	if err := json.Unmarshal(want, &wantNormalized); err != nil {
		t.Fatalf("failed to unmarshal want JSON: %v", err)
	}
	if err := json.Unmarshal(got.Bytes(), &gotNormalized); err != nil {
		t.Fatalf("failed to unmarshal got JSON: %v", err)
	}

	if diff := cmp.Diff(wantNormalized, gotNormalized); diff != "" {
		// For better debugging, marshal the 'got' object back to indented JSON string
		gotJSON, _ := json.MarshalIndent(gotNormalized, "", "  ")
		t.Errorf("OpenAPI spec mismatch for new features (-want +got):\n%s\n\nGot:\n%s", diff, gotJSON)
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

	analyzer, err := NewAnalyzer(s, logger, nil)
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
