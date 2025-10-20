package main

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

func TestCallTrace(t *testing.T) {
	basePrefix := "github.com/podhmo/go-scan/examples/call-trace/testdata"
	testCases := []struct {
		name        string
		targetFunc  string
		pkgPatterns []string
	}{
		{
			name:        "direct_func_call",
			targetFunc:  basePrefix + "/direct/src/mylib.Helper",
			pkgPatterns: []string{"./testdata/direct/src/..."},
		},
		{
			name:        "indirect_func_call",
			targetFunc:  basePrefix + "/indirect/src/mylib.Helper",
			pkgPatterns: []string{"./testdata/indirect/src/..."},
		},
		{
			name:        "no_call",
			targetFunc:  "os.Getenv",
			pkgPatterns: []string{"./testdata/direct/src/..."}, // can use direct data for this
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

			err := run(context.Background(), &buf, logger, tc.targetFunc, tc.pkgPatterns)
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

			got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
			want := strings.ReplaceAll(string(expected), "\r\n", "\n")

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("output does not match golden file %s (-want +got):\n%s", goldenFile, diff)
			}
		})
	}
}
