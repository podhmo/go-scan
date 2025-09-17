package symgotest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// RunResult contains the complete output of a symbolic execution test run.
type RunResult struct {
	// ReturnValue is the object returned by the applied function.
	ReturnValue object.Object
	// Error is any runtime error that occurred during the execution.
	Error error
	// FunctionsCalled is an ordered list of the fully-qualified names of all
	// functions that were symbolically executed. This is only populated if
	// TrackCalls() was enabled on the runner.
	FunctionsCalled []string
}

// Runner is a test utility that simplifies running symbolic execution tests.
type Runner struct {
	t          *testing.T
	files      map[string]string
	setupFunc  func(interp *symgo.Interpreter)
	trackCalls bool
}

// NewRunner creates a new test runner for a simple, single-package test from a single source string.
// It automatically creates a "go.mod" file and ensures the source is part of a 'main' package.
func NewRunner(t *testing.T, source string) *Runner {
	t.Helper()
	if !strings.HasPrefix(strings.TrimSpace(source), "package main") {
		source = "package main\n\n" + source
	}
	files := map[string]string{
		"go.mod":  "module example.com/simple",
		"main.go": source,
	}
	return &Runner{t: t, files: files}
}

// NewRunnerWithMultiFiles creates a new test runner for a complex, multi-file or multi-package test.
// The `files` map must contain a "go.mod" entry.
func NewRunnerWithMultiFiles(t *testing.T, files map[string]string) *Runner {
	t.Helper()
	if _, ok := files["go.mod"]; !ok {
		t.Fatal("NewRunnerWithMultiFiles requires a 'go.mod' file in the files map")
	}
	return &Runner{t: t, files: files}
}

// WithSetup provides a function to configure the symgo.Interpreter before execution.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner {
	r.setupFunc = setupFunc
	return r
}

// TrackCalls enables the recording of all function calls made during the execution.
func (r *Runner) TrackCalls() *Runner {
	r.trackCalls = true
	return r
}

// Apply runs symbolic execution starting from a specific function and returns a rich result object.
func (r *Runner) Apply(funcName string, args ...object.Object) *RunResult {
	r.t.Helper()

	result := &RunResult{}
	ctx := context.Background()

	dir, cleanup := scantest.WriteFiles(r.t, r.files)
	defer cleanup()

	var mainPackagePath string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		var mainPkg *goscan.Package
		for _, p := range pkgs {
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

		if r.trackCalls {
			interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, intrinsicArgs []symgo.Object) symgo.Object {
				if len(intrinsicArgs) > 0 {
					var key string
					fn := intrinsicArgs[0]
					switch fn := fn.(type) {
					case *symgo.Function:
						if fn.Def != nil && fn.Package != nil {
							key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Def.Name)
						}
					case *symgo.SymbolicPlaceholder:
						if fn.UnderlyingFunc != nil && fn.Package != nil {
							key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.UnderlyingFunc.Name)
						}
					}
					if key != "" {
						result.FunctionsCalled = append(result.FunctionsCalled, key)
					}
				}
				// Return a placeholder so execution can continue.
				return &object.SymbolicPlaceholder{Reason: "traced call"}
			})
		}

		for _, p := range pkgs {
			for _, file := range p.AstFiles {
				_, _ = interp.Eval(ctx, file, p)
			}
		}

		fn, ok := interp.FindObjectInPackage(ctx, mainPackagePath, funcName)
		if !ok {
			return fmt.Errorf("function %q not found in package %q", funcName, mainPackagePath)
		}

		// If tracking, manually add the entrypoint to the trace, as Apply() itself doesn't trigger the intrinsic.
		if r.trackCalls {
			fqn := fmt.Sprintf("%s.%s", mainPackagePath, funcName)
			result.FunctionsCalled = append(result.FunctionsCalled, fqn)
		}

		res, err := interp.Apply(ctx, fn, args, mainPkg)
		result.ReturnValue = res
		result.Error = err
		return nil
	}

	if _, err := scantest.Run(r.t, ctx, dir, []string{"./..."}, action); err != nil {
		r.t.Fatalf("scantest.Run() failed: %+v", err)
	}

	return result
}
