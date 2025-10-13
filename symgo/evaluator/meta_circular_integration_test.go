package evaluator_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

// TestMetaCircularAnalysis_MethodCallOnFunctionPointer is a regression test
// for a metacircular analysis bug.
//
// The bug occurred when the evaluator, while analyzing its own code, encountered
// a method call on a pointer to an `*object.Function`. For example, in `accessor.go`,
// the line `boundFn := baseFn.WithReceiver(...)` where `baseFn` is a `*object.Function`,
// would cause a panic. The evaluator would incorrectly treat `baseFn` as a pointer
// to an `*object.Instance` instead of an `*object.Function`, leading to an
// "undefined method or field: WithReceiver for pointer type INSTANCE" error.
//
// This test reproduces the failure by forcing the evaluator to analyze a piece of
// its own code (`(*Accessor).findMethodOnType`) that is known to trigger this
// specific problematic code path.
func TestMetaCircularAnalysis_MethodCallOnFunctionPointer(t *testing.T) {
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		// Remove time to make log output deterministic for string matching.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	testLogger := slog.New(handler)

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		var evaluatorPkg *goscan.Package
		for _, p := range pkgs {
			// We target the evaluator package itself for analysis.
			if p.ImportPath == "github.com/podhmo/go-scan/symgo/evaluator" {
				evaluatorPkg = p
				break
			}
		}
		if evaluatorPkg == nil {
			return fmt.Errorf("package 'github.com/podhmo/go-scan/symgo/evaluator' not found")
		}

		// Create a new evaluator that uses our test logger to capture output.
		eval := evaluator.New(s, testLogger, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		// The core of the test: evaluate all files in the evaluator package.
		// This forces the metacircular analysis.
		for _, file := range evaluatorPkg.AstFiles {
			eval.Eval(ctx, file, env, evaluatorPkg)
		}
		return nil
	}

	// We run the scanner on the project root ("../..") because this test file
	// is in a `_test` package, so we need to go up two levels to find the project root.
	// We specifically target the `symgo/evaluator` package for scanning.
	// scantest.Run will catch any panic and return it as an error.
	_, err := scantest.Run(t, t.Context(), "../..", []string{"github.com/podhmo/go-scan/symgo/evaluator"}, action)

	// After the run, check the captured log output for the specific error message.
	logOutput := logBuf.String()
	expectedErrorMsg := "undefined method or field: WithReceiver for pointer type INSTANCE"

	// The bug can manifest as a panic (err != nil) or a specific logged error.
	hasPanic := err != nil
	hasLoggedError := strings.Contains(logOutput, expectedErrorMsg)

	// If neither a panic nor the specific error log occurred, the test passes.
	if !hasPanic && !hasLoggedError {
		return
	}

	// If we reach here, the bug is present. Fail the test with a detailed message.
	var failureMsg strings.Builder
	failureMsg.WriteString("FAIL: Metacircular analysis bug detected.\n")
	if hasPanic {
		failureMsg.WriteString(fmt.Sprintf("  - Reason: A panic occurred: %v\n", err))
	}
	if hasLoggedError {
		failureMsg.WriteString(fmt.Sprintf("  - Reason: Found expected error message in log: %q\n", expectedErrorMsg))
	}
	failureMsg.WriteString("\n--- Full Log Output ---\n")
	failureMsg.WriteString(logOutput)
	failureMsg.WriteString("\n--- End Log Output ---\n")
	t.Fatal(failureMsg.String())
}