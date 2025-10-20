package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
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
		{
			name:        "method_call",
			targetFunc:  basePrefix + "/method_call/src/mylib.(*Greeter).Greet",
			pkgPatterns: []string{"./testdata/method_call/src/..."},
		},
		{
			name:        "indirect_method_call",
			targetFunc:  basePrefix + "/indirect_method_call/src/mylib.TargetFunc",
			pkgPatterns: []string{"./testdata/indirect_method_call/src/..."},
		},
		{
			name:        "interface_call",
			targetFunc:  basePrefix + "/interface_call/src/mylib.(*greeter).Greet",
			pkgPatterns: []string{"./testdata/interface_call/src/..."},
		},
		{
			name:        "multi_impl",
			targetFunc:  basePrefix + "/multi_impl/src/mylib.(*Japanese).GetMessage",
			pkgPatterns: []string{"./testdata/multi_impl/src/..."},
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

			// Normalization
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

func TestCallTraceOutOfPolicy(t *testing.T) {
	basePrefix := "github.com/podhmo/go-scan/examples/call-trace/testdata"
	tc := struct {
		name        string
		targetFunc  string
		pkgPatterns []string
		policyPkg   string
	}{
		name:       "out_of_policy_import",
		targetFunc: basePrefix + "/out_of_policy/src/mylib.InScope",
		pkgPatterns: []string{
			"./testdata/out_of_policy/src/...", // Scan all packages
		},
		policyPkg: basePrefix + "/out_of_policy/src/anotherlib", // But exclude this one
	}

	t.Run(tc.name, func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

		err := runOutOfPolicyTest(context.Background(), &buf, logger, tc.targetFunc, tc.pkgPatterns, tc.policyPkg)
		if err != nil {
			t.Fatalf("runOutOfPolicyTest() failed: %v", err)
		}

		// Normalization
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("could not get working directory: %v", err)
		}
		rootDir := filepath.Dir(filepath.Dir(wd))

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

func runOutOfPolicyTest(ctx context.Context, out io.Writer, logger *slog.Logger, targetFunc string, pkgPatterns []string, policyExcludePkg string) error {
	s, err := goscan.New(goscan.WithLogger(logger), goscan.WithGoModuleResolver())
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}
	for _, pattern := range pkgPatterns {
		if _, err := s.Scan(ctx, pattern); err != nil {
			return fmt.Errorf("failed to scan packages for pattern %q: %w", pattern, err)
		}
	}

	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(logger.WithGroup("symgo")),
		symgo.WithScanPolicy(func(importPath string) bool {
			return importPath != policyExcludePkg
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
			if calleeName == targetFunc {
				stack := i.CallStack()
				directHits = append(directHits, stack)
			}
		}
		return nil
	})

	mainPkgPath := "github.com/podhmo/go-scan/examples/call-trace/testdata/out_of_policy/src/myapp"
	pkg, ok := s.AllSeenPackages()[mainPkgPath]
	if !ok {
		return fmt.Errorf("main package not found: %s", mainPkgPath)
	}

	var mainFunc *scanner.FunctionInfo
	for _, f := range pkg.Functions {
		if f.Name == "main" {
			mainFunc = f
			break
		}
	}
	if mainFunc == nil {
		return fmt.Errorf("main function not found in package %s", mainPkgPath)
	}

	eval := interp.EvaluatorForTest()
	pkgObj, err := eval.GetOrLoadPackageForTest(ctx, mainPkgPath)
	if err != nil {
		return fmt.Errorf("failed to load package for analysis: %w", err)
	}
	fnObj := eval.GetOrResolveFunctionForTest(ctx, pkgObj, mainFunc)
	interp.Apply(ctx, fnObj, nil, pkg)

	interp.Finalize(ctx)

	// Print results
	if len(directHits) == 0 {
		fmt.Fprintf(out, "No calls to %s found.\n", targetFunc)
		return nil
	}

	fmt.Fprintf(out, "Found %d call stacks to %s:\n\n", len(directHits), targetFunc)
	fset := s.Fset()
	for i, stack := range directHits {
		fmt.Fprintf(out, "--- Stack %d ---\n", i+1)
		for _, frame := range stack {
			fmt.Fprintln(out, frame.Format(fset))
		}
		fmt.Fprintln(out)
	}

	return nil
}
