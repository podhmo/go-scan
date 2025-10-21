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
		name              string
		dir               string
		mainPkg           string
		targetFunc        string
		scanPolicyExclude string
	}{
		{
			name:       "direct_func_call",
			dir:        "./testdata/direct",
			mainPkg:    basePrefix + "/direct/src/myapp",
			targetFunc: basePrefix + "/direct/src/mylib.Helper",
		},
		{
			name:       "indirect_func_call",
			dir:        "./testdata/indirect",
			mainPkg:    basePrefix + "/indirect/src/myapp",
			targetFunc: basePrefix + "/indirect/src/mylib.Helper",
		},
		{
			name:       "no_call",
			dir:        "./testdata/direct", // can use direct data for this
			mainPkg:    basePrefix + "/direct/src/myapp",
			targetFunc: "os.Getenv",
		},
		{
			name:       "method_call",
			dir:        "./testdata/method_call",
			mainPkg:    basePrefix + "/method_call/src/myapp",
			targetFunc: basePrefix + "/method_call/src/mylib.(*Greeter).Greet",
		},
		{
			name:       "indirect_method_call",
			dir:        "./testdata/indirect_method_call",
			mainPkg:    basePrefix + "/indirect_method_call/src/myapp",
			targetFunc: basePrefix + "/indirect_method_call/src/mylib.TargetFunc",
		},
		{
			name:              "out_of_policy_import",
			dir:               "./testdata/out_of_policy",
			mainPkg:           basePrefix + "/out_of_policy/src/myapp",
			targetFunc:        basePrefix + "/out_of_policy/src/mylib.InScope",
			scanPolicyExclude: basePrefix + "/out_of_policy/src/anotherlib",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

			// Call the refactored run function directly.
			err := run(
				context.Background(),
				&buf,
				logger,
				tc.targetFunc,
				[]string{"./src/..."}, // Scan pattern relative to workDir
				tc.dir,                // workDir
				tc.mainPkg,            // mainPkgPath
				tc.scanPolicyExclude,  // scanPolicyExclude
			)
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}

			// Normalization and Golden file comparison
			wd, err := os.Getwd()
			if err != nil {
				t.Fatalf("could not get working directory: %v", err)
			}
			rootDir := filepath.Dir(filepath.Dir(wd)) // project root

			normalizedGot := strings.TrimSpace(buf.String())
			normalizedGot = strings.ReplaceAll(normalizedGot, "\r\n", "\n")
			normalizedGot = strings.ReplaceAll(normalizedGot, rootDir, "##WORKDIR##")
			normalizedGot = strings.ReplaceAll(normalizedGot, "\\", "/")

			goldenFile := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.WriteFile(goldenFile, []byte(normalizedGot), 0644); err != nil {
					t.Fatalf("failed to update golden file %s: %v", goldenFile, err)
				}
				return
			}

			expected, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}
			want := strings.TrimSpace(string(expected))
			want = strings.ReplaceAll(want, "\r\n", "\n")

			if diff := cmp.Diff(want, normalizedGot); diff != "" {
				failedFile := filepath.Join("testdata", tc.name+".failed")
				os.WriteFile(failedFile, []byte(normalizedGot), 0644)
				t.Errorf("output does not match golden file %s (-want +got):\n%s", goldenFile, diff)
				t.Logf("Wrote failing output to %s", failedFile)
			}
		})
	}
}
