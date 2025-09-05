package evaluator

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_SendStmt(t *testing.T) {
	source := `
package main

func getValue() int {
	return 42
}

func getChan() chan int {
	return make(chan int)
}

func main() {
	ch := getChan()
	ch <- getValue()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calledFunctions []string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				if fn, ok := args[0].(*object.Function); ok {
					if fn.Name != nil {
						calledFunctions = append(calledFunctions, fn.Name.Name)
					}
				}
			}
			return &object.SymbolicPlaceholder{Reason: "mocked function call"}
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)

		expected := []string{"getChan", "getValue"}
		if diff := cmp.Diff(expected, calledFunctions); diff != "" {
			return fmt.Errorf("tracked functions mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEval_ChanType(t *testing.T) {
	source := `
package main

func main() {
	var ch chan int
	_ = ch
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)
		if isError(result) {
			return fmt.Errorf("evaluation failed: %v", result.Inspect())
		}

		// Success is not crashing. The change was to prevent a "not implemented" error.
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
