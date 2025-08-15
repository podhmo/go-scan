package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

func TestGenerate(t *testing.T) {
	cases := []struct {
		name       string
		pkgPath    string
		goldenFile string
	}{
		{
			name:       "generics",
		pkgPath:    "./testdata/generics",
			goldenFile: "generics.golden",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
		tmpdir := t.TempDir()

		// The 'run' function expects to be run from the module root.
		// The pkgPath for ScanPackageByImport should be an import path.
		// Let's use the full path from the module root.
		// The module is `github.com/podhmo/go-scan`.
		// The import path is relative to the module root.
		// This is getting complicated. Let's adjust the `run` function's expectation.
		// No, let's adjust the test. The tool is a command-line tool, it should work with relative paths.
		// The pkgPath for ScanPackageByImport should be an import path.
		// Let's assume the test runs from the `minigo-gen-bindings` directory.
		// No, tests run from the package directory.
		// The package path needs to be a proper Go import path.
		// Let's use the full path from the module root.
		// The module is `github.com/podhmo/go-scan`.
		importPath := "github.com/podhmo/go-scan/examples/minigo-gen-bindings/testdata/generics"

		if err := run(context.Background(), tmpdir, []string{importPath}); err != nil {
			t.Fatalf("run() failed unexpectedly: %+v", err)
		}

		generatedFile := filepath.Join(tmpdir, importPath, "install.go")
			generated, err := os.ReadFile(generatedFile)
			if err != nil {
			t.Fatalf("failed to read generated output file %q: %v", generatedFile, err)
			}
			normalizedGenerated := strings.ReplaceAll(string(generated), "\r\n", "\n")

			goldenPath := filepath.Join("testdata", tc.goldenFile)
			if *update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, []byte(normalizedGenerated), 0644); err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				t.Logf("golden file updated: %s", goldenPath)
				return
			}

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}
			normalizedGolden := strings.ReplaceAll(string(golden), "\r\n", "\n")

			if diff := cmp.Diff(normalizedGolden, normalizedGenerated); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
