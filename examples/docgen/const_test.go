package main

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
)

func TestDocgen_withConstantResolution(t *testing.T) {
	moduleDir := "testdata/const-resolution"
	apiPath := "example.com/const-resolution"
	patternsFile := "patterns.go" // Relative to the new CWD

	// Setup: Change directory to the testdata so the module can be resolved.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}
	if err := os.Chdir(moduleDir); err != nil {
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
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "main"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Assert the generated spec directly.
	if apiSpec.Paths == nil {
		t.Fatal("apiSpec.Paths is nil")
	}
	pathItem, ok := apiSpec.Paths["/users"]
	if !ok {
		t.Fatal("/users path not found in spec")
	}
	if pathItem.Get == nil {
		t.Fatal("GET operation not found for /users")
	}

	if len(pathItem.Get.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, but got %d", len(pathItem.Get.Parameters))
	}

	wantParam := &openapi.Parameter{
		Name: "userId",
		In:   "query",
		Schema: &openapi.Schema{
			Type: "string",
		},
	}
	gotParam := pathItem.Get.Parameters[0]

	if diff := cmp.Diff(wantParam, gotParam); diff != "" {
		t.Errorf("parameter mismatch (-want +got):\n%s", diff)
	}
}
