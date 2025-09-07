package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_StructTypeExpression(t *testing.T) {
	source := `
package main

func main() {
	type T struct{ F int }
	var v T
	_ = (struct{ F int })(v)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		// Evaluate the file to populate functions etc.
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		// Get the main function
		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		// Execute main. We expect this to run without "not implemented" errors.
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %v", err)
		}
		if ret, ok := result.(*object.ReturnValue); ok {
			if err, ok := ret.Value.(*object.Error); ok {
				return fmt.Errorf("evaluation returned an error: %v", err)
			}
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
