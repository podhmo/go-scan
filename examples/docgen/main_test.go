package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	goscan "github.com/podhmo/go-scan"
)

func TestDocgen(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	// Setup
	s, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	analyzer, err := NewAnalyzer(s)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Run analysis
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}

	// Verification
	want := &openapi.OpenAPI{
		OpenAPI: "3.1.0",
		Info: openapi.Info{
			Title:   "Sample API",
			Version: "0.0.1",
		},
		Paths: map[string]*openapi.PathItem{
			"/users": {
				Get: &openapi.Operation{
					OperationID: "listUsers",
					Description: "listUsers handles the GET /users endpoint.\nIt returns a list of all users.",
				},
			},
		},
	}

	// Normalize description before comparison
	got := analyzer.OpenAPI
	if got.Paths["/users"] != nil && got.Paths["/users"].Get != nil {
		got.Paths["/users"].Get.Description = strings.TrimSpace(got.Paths["/users"].Get.Description)
	}


	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("OpenAPI spec mismatch (-want +got):\n%s", diff)
	}
}
