package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scanner"
)

var update = flag.Bool("update", false, "update golden files")

func TestGoInspect(t *testing.T) {
	testCases := []struct {
		name              string
		pkgPatterns       []string
		withPatterns      []string
		targets           []string
		trimPrefix        bool
		includeUnexported bool
		shortFormat       bool
		expandFormat      bool
	}{
		{
			name:        "default",
			pkgPatterns: []string{"./testdata/src/myapp"},
		},
		{
			name:        "short",
			pkgPatterns: []string{"./testdata/src/myapp"},
			shortFormat: true,
		},
		{
			name:         "expand",
			pkgPatterns:  []string{"./testdata/src/myapp"},
			expandFormat: true,
		},
		{
			name:              "unexported",
			pkgPatterns:       []string{"./testdata/src/myapp"},
			includeUnexported: true,
		},
		{
			name:              "accessor",
			pkgPatterns:       []string{"./testdata/src/features"},
			includeUnexported: true, // To see setters on unexported fields
		},
		{
			name:        "cross_package",
			pkgPatterns: []string{"./testdata/src/features"},
		},
		{
			name:         "multi_package_expand",
			pkgPatterns:  []string{"./testdata/src/..."},
			expandFormat: true,
		},
		{
			name:        "toplevel",
			pkgPatterns: []string{"./testdata/src/toplevel"},
		},
		{
			name:        "target_func_a",
			pkgPatterns: []string{"./testdata/src/target"},
			targets:     []string{"github.com/podhmo/go-scan/tools/goinspect/testdata/src/target.FuncA"},
		},
		{
			name:        "trim_prefix",
			pkgPatterns: []string{"./testdata/src/myapp"},
			trimPrefix:  true,
		},
		{
			name:        "multi_pkg_target",
			pkgPatterns: []string{"./testdata/src/myapp", "./testdata/src/another"},
		},
		{
			name:        "mutual",
			pkgPatterns: []string{"./testdata/src/mutual"},
		},
		{
			name:        "indirect",
			pkgPatterns: []string{"./testdata/src/indirect"},
		},
		{
			name:        "stdlib_errors",
			pkgPatterns: []string{"errors"},
		},
		{
			name:         "with_option",
			pkgPatterns:  []string{"./testdata/src/myapp"},
			withPatterns: []string{"./testdata/src/another"},
		},
		{
			name:        "special_funcs",
			pkgPatterns: []string{"./testdata/src/special/..."},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running test case %s", tc.name)

			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			// Set parallelism to 1 for deterministic output ordering in tests
			ctx := context.Background()
			ctx = scanner.WithParallelismLimit(ctx, 1)

			err := run(ctx, &buf, logger, tc.pkgPatterns, tc.withPatterns, tc.targets, tc.trimPrefix, tc.includeUnexported, tc.shortFormat, tc.expandFormat)
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}

			goldenFile := filepath.Join("testdata", tc.name+".golden")
			if *update {
				err := os.WriteFile(goldenFile, buf.Bytes(), 0644)
				if err != nil {
					t.Fatalf("failed to update golden file %s: %v", goldenFile, err)
				}
				return
			}

			expected, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}

			// Normalize line endings for comparison
			got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
			want := strings.ReplaceAll(string(expected), "\r\n", "\n")

			if got != want {
				t.Errorf("output does not match golden file %s\n", goldenFile)
				t.Logf("GOT:\n%s", got)
				t.Logf("WANT:\n%s", want)
			}
		})
	}
}

func TestGoInspect_NoModuleContext(t *testing.T) {
	// This test simulates running goinspect from a directory without a go.mod file.

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer os.Chdir(originalWd)

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory to temp dir: %v", err)
	}
	t.Logf("changed working directory to %s", tmpDir)

	testCases := []struct {
		name              string
		pkgPatterns       []string
		withPatterns      []string
		targets           []string
		trimPrefix        bool
		includeUnexported bool
		shortFormat       bool
		expandFormat      bool
	}{
		{
			name:        "no_module_stdlib_fmt",
			pkgPatterns: []string{"fmt"},
			targets:     []string{"fmt.Println"},
			shortFormat: true,
		},
		{
			name:        "no_module_external_go_cmp",
			pkgPatterns: []string{"github.com/google/go-cmp/cmp"},
			targets:     []string{"github.com/google/go-cmp/cmp.Diff"},
			shortFormat: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running test case %s from outside a module", tc.name)

			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			ctx := context.Background()
			ctx = scanner.WithParallelismLimit(ctx, 1)

			err := run(ctx, &buf, logger, tc.pkgPatterns, tc.withPatterns, tc.targets, tc.trimPrefix, tc.includeUnexported, tc.shortFormat, tc.expandFormat)
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}

			goldenFile := filepath.Join(originalWd, "testdata", tc.name+".golden")

			if *update {
				err := os.WriteFile(goldenFile, buf.Bytes(), 0644)
				if err != nil {
					t.Fatalf("failed to update golden file %s: %v", goldenFile, err)
				}
				t.Logf("updated golden file: %s", goldenFile)
				return
			}

			expected, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}

			got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
			want := strings.ReplaceAll(string(expected), "\r\n", "\n")

			if got != want {
				t.Errorf("output does not match golden file %s\n", goldenFile)
				t.Logf("GOT:\n%s", got)
				t.Logf("WANT:\n%s", want)
			}
		})
	}
}
