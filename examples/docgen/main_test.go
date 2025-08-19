package main

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
)

func TestIt(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	s, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	pkg, err := s.ScanPackageByImport(context.Background(), sampleAPIPath)
	if err != nil {
		t.Fatalf("failed to load sample API package: %v", err)
	}

	if pkg == nil {
		t.Fatal("loaded package should not be nil")
	}

	if pkg.Name != "sampleapi" {
		t.Errorf("expected package name to be 'sampleapi', but got %q", pkg.Name)
	}
}
