package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoInspect(t *testing.T) {
	testCases := []struct {
		name              string
		pkgPattern        string
		includeUnexported bool
		shortFormat       bool
		expandFormat      bool
	}{
		{
			name:       "default",
			pkgPattern: "./testdata/src/myapp",
		},
		{
			name:       "short",
			pkgPattern: "./testdata/src/myapp",
			shortFormat: true,
		},
		{
			name:       "expand",
			pkgPattern: "./testdata/src/myapp",
			expandFormat: true,
		},
		{
			name:              "unexported",
			pkgPattern:        "./testdata/src/myapp",
			includeUnexported: true,
		},
		{
			name:              "accessor",
			pkgPattern:        "./testdata/src/features",
			includeUnexported: true, // To see setters on unexported fields
		},
		{
			name:       "cross_package",
			pkgPattern: "./testdata/src/features",
		},
		{
			name:         "multi_package_expand",
			pkgPattern:   "./testdata/src/...",
			expandFormat: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			err := run(&buf, logger, tc.pkgPattern, tc.includeUnexported, tc.shortFormat, tc.expandFormat)
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}

			goldenFile := filepath.Join("testdata", tc.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") != "" {
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