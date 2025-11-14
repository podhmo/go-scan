package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenschema(t *testing.T) {
	// Build the genschema binary to a temporary location.
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "genschema")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build genschema binary: %v", err)
	}

	testCases := []struct {
		name        string
		query       string
		goldenFile  string
		title       string
		description string
		loose       bool
	}{
		{
			name:        "Person",
			query:       "github.com/podhmo/go-scan/tools/genschema/testdata.Person",
			goldenFile:  "testdata/person.golden.json",
			title:       "Person Schema",
			description: "Schema for a Person",
			loose:       false,
		},
		{
			name:        "Product",
			query:       "github.com/podhmo/go-scan/tools/genschema/testdata.Product",
			goldenFile:  "testdata/product.golden.json",
			title:       "Product Schema",
			description: "Schema for a Product",
			loose:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"--query", tc.query,
				"--schema-title", tc.title,
				"--schema-description", tc.description,
			}
			if tc.loose {
				args = append(args, "--loose")
			} else {
				// explicitly set loose=false to match golden files
				args = append(args, "--loose=false")
			}

			cmd := exec.Command(binPath, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err != nil {
				t.Fatalf("genschema command failed: %v\nStderr: %s", err, stderr.String())
			}

			// Load the golden file.
			goldenData, err := os.ReadFile(tc.goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %q: %v", tc.goldenFile, err)
			}

			// Unmarshal and remarshal to normalize JSON formatting (indentation, etc.)
			var actualJSON, expectedJSON bytes.Buffer
			if err := json.Indent(&actualJSON, stdout.Bytes(), "", "  "); err != nil {
				t.Fatalf("failed to format actual output: %v", err)
			}
			if err := json.Indent(&expectedJSON, goldenData, "", "  "); err != nil {
				t.Fatalf("failed to format golden file: %v", err)
			}

			if diff := cmp.Diff(expectedJSON.String(), actualJSON.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
