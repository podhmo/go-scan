package main

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "update golden files")

func TestDocgen(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	// Setup
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

	// Run analysis
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath, "NewServeMux"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}

	// Marshal the result to YAML
	var got bytes.Buffer
	enc := yaml.NewEncoder(&got)
	if err := enc.Encode(analyzer.OpenAPI); err != nil {
		t.Fatalf("failed to encode OpenAPI spec to YAML: %v", err)
	}

	// Golden file testing
	goldenFile := filepath.Join("testdata", "golden.yaml")
	if *update {
		if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("golden file updated: %s", goldenFile)
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if diff := cmp.Diff(string(want), got.String()); diff != "" {
		t.Errorf("OpenAPI spec mismatch (-want +got):\n%s", diff)
	}
}
