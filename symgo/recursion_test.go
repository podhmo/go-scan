package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestRecursionWithMultiReturn(t *testing.T) {
	// This test case reproduces the infinite hang that occurs when the evaluator
	// encounters a recursive function with multiple return values.
	// The `Get` function below simulates the structure of `minigo.object.Environment.Get`.
	source := `
package main

type Env struct {
	Outer *Env
}

func (e *Env) Get(name string) (any, bool) {
	if e.Outer != nil {
		return e.Outer.Get(name) // Recursive call
	}
	return nil, false
}

func main() {
	env := &Env{Outer: &Env{}}
	env.Get("foo")
}
`
	// 1. Define the test module's file layout.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/recursion",
		"main.go": source,
	})
	defer cleanup()

	// 2. Define the test logic in an action function.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		// Evaluate the file to define symbols.
		_, err = interp.Eval(ctx, pkg.AstFiles[pkg.Files[0]], pkg)
		if err != nil {
			return fmt.Errorf("file-level eval failed: %w", err)
		}

		// Execute main.
		mainFunc, ok := interp.FindObject("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		_, err = interp.Apply(ctx, mainFunc, []symgo.Object{}, pkg)
		if err != nil {
			// The original bug would cause a timeout here.
			// With the fix, it should complete without error.
			return fmt.Errorf("apply main failed: %w", err)
		}

		return nil
	}

	// 3. Use scantest.Run to drive the test.
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
