package symgotest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

const (
	moduleName = "example.com/symgotest/module"
)

// Runner is a test utility to simplify running symbolic execution tests.
type Runner struct {
	t         *testing.T
	source    string
	setupFunc func(interp *symgo.Interpreter)
}

// NewRunner creates a new test runner for a given source code snippet.
// If the source does not already contain a "package main" declaration, it will be added.
func NewRunner(t *testing.T, source string) *Runner {
	t.Helper()
	if !strings.HasPrefix(strings.TrimSpace(source), "package main") {
		source = "package main\n\n" + source
	}
	return &Runner{t: t, source: source}
}

// WithSetup provides a function to configure the symgo.Interpreter before execution.
// This is primarily used to register intrinsic functions for mocking.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner {
	r.setupFunc = setupFunc
	return r
}

// Apply runs symbolic execution starting from a specific function in the main package.
// It handles all the boilerplate of setting up a temporary module, scanning,
// and creating an interpreter.
//
// It returns the object.Object that results from the function application.
func (r *Runner) Apply(funcName string, args ...object.Object) object.Object {
	r.t.Helper()

	// Use a struct to hold the result from the scantest.ActionFunc
	var result struct {
		val object.Object
	}

	dir, cleanup := scantest.WriteFiles(r.t, map[string]string{
		"go.mod":  fmt.Sprintf("module %s", moduleName),
		"main.go": r.source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) == 0 {
			return fmt.Errorf("no packages were scanned")
		}
		mainPkg := pkgs[0]

		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		if r.setupFunc != nil {
			r.setupFunc(interp)
		}

		// Evaluate the file to process imports and top-level declarations.
		mainFilePath := filepath.Join(dir, "main.go")
		if _, err := interp.Eval(ctx, mainPkg.AstFiles[mainFilePath], mainPkg); err != nil {
			// An error during initial evaluation might be expected, so we don't fail hard here.
			// The subsequent 'Apply' will likely fail with a more specific error.
		}

		// Find the entry point function.
		fn, ok := interp.FindObjectInPackage(ctx, moduleName, funcName)
		if !ok {
			return fmt.Errorf("function %q not found in package %q", funcName, moduleName)
		}

		// Apply the function.
		res, err := interp.Apply(ctx, fn, args, mainPkg)
		if err != nil {
			// Wrap apply errors in an object.Error to be consistent.
			result.val = &object.Error{Message: err.Error()}
		} else {
			result.val = res
		}

		return nil
	}

	if _, err := scantest.Run(r.t, context.Background(), dir, []string{"."}, action); err != nil {
		r.t.Fatalf("scantest.Run() failed: %+v", err)
	}

	return result.val
}
