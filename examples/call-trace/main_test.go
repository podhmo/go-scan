package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

var update = flag.Bool("update", false, "update golden files")

func TestCallTrace(t *testing.T) {
	basePrefix := "github.com/podhmo/go-scan/examples/call-trace/testdata"

	testCases := []struct {
		name              string
		dir               string
		mainPkg           string
		targetFunc        string
		scanPolicyExclude string // PkgPath to exclude from scan policy
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

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				// Most of the logic from the original `run` function goes here.
				interp, err := symgo.NewInterpreter(s,
					symgo.WithLogger(logger.WithGroup("symgo")),
					symgo.WithScanPolicy(func(importPath string) bool {
						return importPath != tc.scanPolicyExclude
					}),
				)
				if err != nil {
					return fmt.Errorf("failed to create interpreter: %w", err)
				}

				var directHits [][]*object.CallFrame
				interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
					calleeObj := args[0]
					var calleeFunc *scanner.FunctionInfo

					switch f := calleeObj.(type) {
					case *object.Function:
						calleeFunc = f.Def
					case *object.SymbolicPlaceholder:
						calleeFunc = f.UnderlyingFunc
					}

					if calleeFunc != nil {
						calleeName := getFuncTargetName(calleeFunc)
						if calleeName == tc.targetFunc {
							stack := i.CallStack()
							directHits = append(directHits, stack)
						}
					}
					return nil
				})

				pkg, ok := s.AllSeenPackages()[tc.mainPkg]
				if !ok {
					return fmt.Errorf("main package not found: %s", tc.mainPkg)
				}

				var mainFunc *scanner.FunctionInfo
				for _, f := range pkg.Functions {
					if f.Name == "main" {
						mainFunc = f
						break
					}
				}
				if mainFunc == nil {
					return fmt.Errorf("main function not found in package %s", tc.mainPkg)
				}

				eval := interp.EvaluatorForTest()
				pkgObj, err := eval.GetOrLoadPackageForTest(ctx, tc.mainPkg)
				if err != nil {
					return fmt.Errorf("failed to load package for analysis: %w", err)
				}
				fnObj := eval.GetOrResolveFunctionForTest(ctx, pkgObj, mainFunc)
				interp.Apply(ctx, fnObj, nil, pkg)

				interp.Finalize(ctx)

				// Print results to buffer
				if len(directHits) == 0 {
					fmt.Fprintf(&buf, "No calls to %s found.\n", tc.targetFunc)
					return nil
				}

				fmt.Fprintf(&buf, "Found %d call stacks to %s:\n\n", len(directHits), tc.targetFunc)
				fset := s.Fset()
				for i, stack := range directHits {
					fmt.Fprintf(&buf, "--- Stack %d ---\n", i+1)
					for _, frame := range stack {
						fmt.Fprintln(&buf, frame.Format(fset))
					}
					fmt.Fprintln(&buf)
				}
				return nil
			}

			// scantest.Run will set up the scanner with go.mod support.
			// The pattern is relative to tc.dir.
			_, err := scantest.Run(t, context.Background(), tc.dir, []string{"./..."}, action)
			if err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
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
