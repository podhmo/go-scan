package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

var update = flag.Bool("update", false, "update golden files")

// loadTestdata reads all files from a directory recursively and returns them as a map
// with relative paths as keys, suitable for scantest.WriteFiles.
func loadTestdata(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to load testdata from %s: %v", root, err)
	}
	return files
}

func TestRun(t *testing.T) {
	testdataFiles := loadTestdata(t, "testdata/walk")

	cases := []struct {
		name       string
		args       map[string]interface{}
		goldenFile string
	}{
		{
			name: "default-hops=1-in-module",
			args: map[string]interface{}{
				"start-pkg": "github.com/podhmo/go-scan/testdata/walk/a",
				"hops":      1,
				"full":      false,
				"short":     false,
				"ignore":    "",
			},
			goldenFile: "default.golden",
		},
		{
			name: "default-short",
			args: map[string]interface{}{
				"start-pkg": "github.com/podhmo/go-scan/testdata/walk/a",
				"hops":      1,
				"full":      false,
				"short":     true,
				"ignore":    "",
			},
			goldenFile: "default-short.golden",
		},
		{
			name: "hops=2",
			args: map[string]interface{}{
				"start-pkg": "github.com/podhmo/go-scan/testdata/walk/a",
				"hops":      2,
				"full":      true, // to see external deps
				"short":     false,
				"ignore":    "",
			},
			goldenFile: "hops2.golden",
		},
		{
			name: "ignore-c",
			args: map[string]interface{}{
				"start-pkg": "github.com/podhmo/go-scan/testdata/walk/a",
				"hops":      1,
				"full":      false,
				"short":     false,
				"ignore":    "github.com/podhmo/go-scan/testdata/walk/c",
			},
			goldenFile: "ignore-c.golden",
		},
		{
			name: "full",
			args: map[string]interface{}{
				"start-pkg": "github.com/podhmo/go-scan/testdata/walk/d", // d imports an external package
				"hops":      1,
				"full":      true,
				"short":     false,
				"ignore":    "",
			},
			goldenFile: "full.golden",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir, cleanup := scantest.WriteFiles(t, testdataFiles)
			defer cleanup()

			originalWD, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get wd: %v", err)
			}
			if err := os.Chdir(tmpdir); err != nil {
				t.Fatalf("failed to change wd to tmpdir: %v", err)
			}
			defer os.Chdir(originalWD)

			outputFile := filepath.Join(tmpdir, "output.dot")
			startPkg := tc.args["start-pkg"].(string)

			err = run(
				context.Background(),
				startPkg,
				tc.args["hops"].(int),
				tc.args["ignore"].(string),
				outputFile,
				tc.args["full"].(bool),
				tc.args["short"].(bool),
			)
			if err != nil {
				t.Fatalf("run() failed unexpectedly: %+v", err)
			}

			generated, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("failed to read generated output file: %v", err)
			}

			goldenPath := filepath.Join(originalWD, "testdata", tc.goldenFile)
			if *update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, generated, 0644); err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				t.Logf("golden file updated: %s", goldenPath)
				return
			}

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}

			normalizedGenerated := strings.ReplaceAll(string(generated), "\r\n", "\n")
			normalizedGolden := strings.ReplaceAll(string(golden), "\r\n", "\n")

			if diff := cmp.Diff(normalizedGolden, normalizedGenerated); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
