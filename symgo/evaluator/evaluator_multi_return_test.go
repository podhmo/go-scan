package evaluator

import (
	"bytes"
	"context"
	"go/token"
	"log/slog"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestEval_MultiReturnAssignment_FromUnscannablePackage(t *testing.T) {
	source := `
package main

import "example.com/fake"

func main() {
	// This function call is the target of our test.
	// "example.com/fake" is not provided, so symgo will treat it as unscannable.
	// The Do() function is expected to return two values.
	_, err := fake.Do()
	_ = err
}
`
	// Setup a logger with a buffer to capture output.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// The action to be performed by scantest.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		eval := New(s, logger, nil, func(path string) bool {
			// Define a scan policy: only the main package is scanned from source.
			// "example.com/fake" will be treated as external.
			return path == "example.com/main"
		})

		// Run the evaluator on the main package.
		mainPkg := pkgs[0]

		// This will populate the package-level declarations (including main) into the evaluator's cache.
		pkgObj, err := eval.getOrLoadPackage(ctx, mainPkg.ImportPath)
		if err != nil {
			return err
		}

		// Find the main function and evaluate it.
		mainFunc, ok := pkgObj.Env.Get("main")
		if !ok {
			t.Fatalf("main function not found in package %q", mainPkg.ImportPath)
		}

		eval.applyFunction(ctx, mainFunc, nil, mainPkg, pkgObj.Env, token.NoPos)
		return nil
	}

	// Use scantest to run the test with an in-memory file system.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": source,
	})
	defer cleanup()

	// Run the test. We expect it to pass without errors from the evaluator itself.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	// Now, check the log output.
	// The bug we are fixing would cause a "expected multi-return value" warning.
	// A successful run should have an empty log buffer (at the WARN level).
	logOutput := buf.String()
	if strings.Contains(logOutput, "expected multi-return value on RHS of assignment") {
		t.Errorf("test failed as expected, found unwanted warning in log:\n---\n%s\n---", logOutput)
	}
}
