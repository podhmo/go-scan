package main

import (
	"bytes"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestGoInspect(t *testing.T) {
	testCases := []struct {
		name              string
		pkgPatterns       []string
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
			targets:     []string{"github.com/podhmo/go-scan/examples/goinspect/testdata/src/target.FuncA"},
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running test case %s", tc.name)

			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			err := run(&buf, logger, tc.pkgPatterns, tc.targets, tc.trimPrefix, tc.includeUnexported, tc.shortFormat, tc.expandFormat)
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
