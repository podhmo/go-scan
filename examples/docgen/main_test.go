package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "update golden files")

func TestDocgen(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	// Setup: Run the analysis once and reuse the result for all sub-tests.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s, err := goscan.New(
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

	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Load custom patterns from the config file.
	// Note: The path is relative to the new CWD.
	customPatterns, err := LoadPatternsFromConfig("patterns.go", logger)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Create an analyzer with the custom patterns.
	analyzer, err := NewAnalyzer(s, logger, customPatterns...)
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

	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Load custom patterns from the config file.
	customPatterns, err := LoadPatternsFromConfig("patterns.go", logger)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Create an analyzer with the custom patterns.
	analyzer, err := NewAnalyzer(s, logger, customPatterns...)
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
