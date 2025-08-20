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
