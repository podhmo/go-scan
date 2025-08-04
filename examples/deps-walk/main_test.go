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
			name: "default-dot",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "default.golden",
		},
		{
			name: "relative-path-dot",
			args: map[string]interface{}{
				"start-pkgs": []string{"./a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "default.golden",
		},
		{
			name: "default-mermaid",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "mermaid",
				"full":       false,
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "default-mermaid.golden",
		},
		{
			name: "default-json",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "json",
				"full":       false,
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "default-json.golden",
		},
		{
			name: "default-json",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "json",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "forward",
			},
			goldenFile: "default-json.golden",
		},
		{
			name: "reverse-json",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/c"},
				"hops":       1,
				"format":     "json",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "reverse",
			},
			goldenFile: "reverse-json.golden",
		},
		{
			name: "bidi-json",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/b"},
				"hops":       1,
				"format":     "json",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "bidi",
			},
			goldenFile: "bidi-json.golden",
		},
		{
			name: "mermaid-short",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "mermaid",
				"full":       false,
				"short":      true,
				"ignore":     "",
			},
			goldenFile: "default-mermaid-short.golden",
		},
		{
			name: "default-short",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      true,
				"ignore":     "",
			},
			goldenFile: "default-short.golden",
		},
		{
			name: "hops=2",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       2,
				"format":     "dot",
				"full":       true, // to see external deps
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "hops2.golden",
		},
		{
			name: "ignore-c",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "github.com/podhmo/go-scan/testdata/walk/c",
			},
			goldenFile: "ignore-c.golden",
		},
		{
			name: "ignore-c-short",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      true,
				"ignore":     "testdata/walk/c",
			},
			goldenFile: "ignore-c-short.golden",
		},
		{
			name: "full",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/d"}, // d imports an external package
				"hops":       1,
				"format":     "dot",
				"full":       true,
				"short":      false,
				"ignore":     "",
			},
			goldenFile: "full.golden",
		},
		{
			name: "file-granularity",
			args: map[string]interface{}{
				"start-pkgs":  []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":        1,
				"format":      "dot",
				"granularity": "file",
				"full":        false,
				"short":       false,
				"ignore":      "",
			},
			goldenFile: "file-granularity.golden",
		},
		{
			name: "reverse",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/c"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "reverse",
			},
			goldenFile: "reverse.golden",
		},
		{
			name: "bidi",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/b"},
				"hops":       1, // Hops apply to forward search
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "bidi",
			},
			goldenFile: "bidi.golden",
		},
		{
			name: "reverse-hops2",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/c"},
				"hops":       2,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "reverse",
			},
			goldenFile: "reverse-hops2.golden",
		},
		{
			name: "reverse-hops2-aggressive",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/c"},
				"hops":       2,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"direction":  "reverse",
				"aggressive": true,
			},
			goldenFile: "reverse-hops2-aggressive.golden",
		},
		{
			name: "with-test-files",
			args: map[string]interface{}{
				"start-pkgs": []string{"github.com/podhmo/go-scan/testdata/walk/a"},
				"hops":       1,
				"format":     "dot",
				"full":       false,
				"short":      false,
				"ignore":     "",
				"test":       true,
			},
			goldenFile: "with-test-files.golden",
		},
		{
			name: "multiple-packages",
			args: map[string]interface{}{
				"start-pkgs": []string{
					"github.com/podhmo/go-scan/testdata/walk/a",
					"github.com/podhmo/go-scan/testdata/walk/d",
				},
				"hops":   1,
				"format": "dot",
				"full":   true, // to see d's external dep
				"short":  false,
				"ignore": "",
			},
			goldenFile: "multiple.golden",
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

			// For aggressive tests, we need a git repo
			if aggressive, ok := tc.args["aggressive"].(bool); ok && aggressive {
				scantest.RunCommand(t, tmpdir, "git", "init")
				scantest.RunCommand(t, tmpdir, "git", "config", "user.email", "you@example.com")
				scantest.RunCommand(t, tmpdir, "git", "config", "user.name", "Your Name")
				scantest.RunCommand(t, tmpdir, "git", "add", ".")
				scantest.RunCommand(t, tmpdir, "git", "commit", "-m", "initial commit")
			}

			// For tests that involve external dependencies, run `go mod tidy`
			if full, ok := tc.args["full"].(bool); ok && full {
				scantest.RunCommand(t, tmpdir, "go", "mod", "tidy")
			}

			format, ok := tc.args["format"].(string)
			if !ok {
				format = "dot" // default format
			}

			outputFile := filepath.Join(tmpdir, "output."+format)
			startPkgs := tc.args["start-pkgs"].([]string)

			granularity, ok := tc.args["granularity"].(string)
			if !ok {
				granularity = "package"
			}

			direction, ok := tc.args["direction"].(string)
			if !ok {
				direction = "forward"
			}

			aggressive, ok := tc.args["aggressive"].(bool)
			if !ok {
				aggressive = false
			}
			test, ok := tc.args["test"].(bool)
			if !ok {
				test = false
			}

			err = run(
				context.Background(),
				startPkgs,
				tc.args["hops"].(int),
				tc.args["ignore"].(string),
				outputFile,
				format,
				granularity,
				tc.args["full"].(bool),
				tc.args["short"].(bool),
				direction,
				aggressive,
				test,
			)
			if err != nil {
				t.Fatalf("run() failed unexpectedly: %+v", err)
			}

			generated, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("failed to read generated output file: %v", err)
			}

			normalizedGenerated := strings.ReplaceAll(string(generated), "\r\n", "\n")

			// Normalize temporary directory paths in file granularity tests
			if tc.name == "file-granularity" {
				// In the generated output, replace the temp dir path with a stable placeholder.
				normalizedGenerated = strings.ReplaceAll(normalizedGenerated, tmpdir, "<tmp>")
			}

			goldenPath := filepath.Join(originalWD, "testdata", tc.goldenFile)
			if *update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				// When updating, write the *normalized* content to the golden file.
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
