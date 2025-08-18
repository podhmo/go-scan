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
		{
			name:       "varsandtypes",
			pkgPath:    "./testdata/varsandtypes",
			goldenFile: "varsandtypes.golden",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir := t.TempDir()
			if err := runGenBindingsInternal(context.Background(), tmpdir, []string{tc.pkgPath}); err != nil {
				t.Fatalf("runGenBindingsInternal() failed unexpectedly: %+v", err)
			}

			generatedFile := filepath.Join(tmpdir, tc.pkgPath, "install.go")
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
