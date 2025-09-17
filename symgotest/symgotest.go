package symgotest

import (
	"context"
	"fmt"
	"go/parser"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

const (
	moduleName = "example.com/symgotest/module"
)

// Runner is a test utility that simplifies running symbolic execution tests on a package.
type Runner struct {
	t         *testing.T
	files     map[string]string
	setupFunc func(interp *symgo.Interpreter)
}

// NewRunner creates a new test runner for a given single-file source code snippet.
// It automatically ensures the source is part of a 'main' package.
func NewRunner(t *testing.T, source string) *Runner {
	t.Helper()
	if !strings.HasPrefix(strings.TrimSpace(source), "package main") {
		source = "package main\n\n" + source
	}
	files := map[string]string{
		"go.mod":  fmt.Sprintf("module %s", moduleName),
		"main.go": source,
	}
	return &Runner{t: t, files: files}
}

// NewRunnerWithMultiFiles creates a new test runner for a multi-file package.
// The `files` map should contain the file paths (relative to the module root) and their content.
// It must include a "go.mod" file.
func NewRunnerWithMultiFiles(t *testing.T, files map[string]string) *Runner {
	t.Helper()
	if _, ok := files["go.mod"]; !ok {
		t.Fatal("NewRunnerWithMultiFiles requires a 'go.mod' file in the files map")
	}
	return &Runner{t: t, files: files}
}

// WithSetup provides a function to configure the symgo.Interpreter before execution.
// This is primarily used to register intrinsic functions for mocking.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner {
	r.setupFunc = setupFunc
	return r
}

// Apply runs symbolic execution starting from a specific function.
// It handles all the boilerplate of setting up a temporary module, scanning,
// and creating an interpreter.
func (r *Runner) Apply(funcName string, args ...object.Object) object.Object {
	r.t.Helper()

	var result object.Object
	ctx := context.Background()

	dir, cleanup := scantest.WriteFiles(r.t, r.files)
	defer cleanup()

	// The full module name for the main package can vary if it's in a subdirectory.
	// We determine it by finding the main package after scanning.
	var mainPackagePath string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Find main package to determine its full import path and to use it for Apply.
		var mainPkg *goscan.Package
		for _, p := range pkgs {
			// A simple heuristic for finding the main package.
			// This might need refinement if multiple main packages exist in complex setups.
			for _, f := range p.AstFiles {
				if f.Name.Name == "main" {
					mainPkg = p
					mainPackagePath = p.ImportPath
					break
				}
			}
			if mainPkg != nil {
				break
			}
		}
		if mainPkg == nil {
			return fmt.Errorf("could not find main package in scanned files")
		}

		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		if r.setupFunc != nil {
			r.setupFunc(interp)
		}

		// Evaluate all files in all scanned packages to populate environments.
		for _, p := range pkgs {
			for _, file := range p.AstFiles {
				if _, err := interp.Eval(ctx, file, p); err != nil {
					// An error during initial evaluation might be expected, so we don't fail hard here.
				}
			}
		}

		// Find the entry point function in the main package.
		fn, ok := interp.FindObjectInPackage(ctx, mainPackagePath, funcName)
		if !ok {
			return fmt.Errorf("function %q not found in package %q", funcName, mainPackagePath)
		}

		// Apply the function.
		res, err := interp.Apply(ctx, fn, args, mainPkg)
		if err != nil {
			result = &object.Error{Message: err.Error()}
		} else {
			result = res
		}
		return nil
	}

	// Scan all packages from the module root.
	if _, err := scantest.Run(r.t, ctx, dir, []string{"./..."}, action); err != nil {
		r.t.Fatalf("scantest.Run() failed: %+v", err)
	}

	return result
}

// EvalExpr parses and evaluates a single Go expression string.
// It's a lightweight helper for testing the evaluation of simple constructs
// without needing a full package context.
func EvalExpr(t *testing.T, expr string) object.Object {
	t.Helper()
	node, err := parser.ParseExpr(expr)
	if err != nil {
		t.Fatalf("failed to parse expression %q: %v", expr, err)
	}

	// For simple expression evaluation, we typically don't need a full scanner.
	eval := evaluator.New(nil, nil, nil, nil)
	return eval.Eval(context.Background(), node, eval.UniverseEnv, nil)
}
