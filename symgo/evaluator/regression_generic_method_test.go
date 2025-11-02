package evaluator_test

import (
	"context"
	"go/ast"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestRegressionGenericMethodCall(t *testing.T) {
	// This is a regression test for a bug where method calls on generic
	// type instances would cause a crash.
	// The evaluator was returning an *object.INSTANCE for generic methods,
	// but the calling code did not handle it, leading to a crash.
	// This test ensures that such method calls are now handled correctly
	// and do not produce an error.

	source := `
package main

type G[T any] struct {
	V T
}

func (g *G[T]) Do() T {
	return g.V
}

func main() {
	g := &G[int]{V: 10}
	g.Do()
}
`
	files := map[string]string{
		"go.mod":  "module my-test",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			t.Fatalf("expected 1 package, but got %d", len(pkgs))
		}
		mainPkg := pkgs[0]

		var mainFile *ast.File
		for _, f := range mainPkg.AstFiles {
			mainFile = f
			break
		}
		if mainFile == nil {
			t.Fatal("main.go AST not found")
		}

		eval := evaluator.New(s, nil, nil, func(pkgpath string) bool { return true })

		// 1. Evaluate the file to populate the evaluator's internal package environment.
		if res := eval.Eval(ctx, mainFile, object.NewEnvironment(), mainPkg); res != nil {
			if err, ok := res.(*object.Error); ok {
				t.Fatalf("Initial Eval failed unexpectedly: %+v", err)
			}
		}

		// 2. Get the package's environment from the evaluator.
		pkgEnv, ok := eval.PackageEnvForTest(mainPkg.ImportPath)
		if !ok {
			t.Fatal("package environment not found after eval")
		}

		// 3. Get the main function from the package environment.
		mainFn, ok := pkgEnv.Get("main")
		if !ok {
			t.Fatal("main function not found in package environment")
		}

		// 4. Apply the main function to trigger the method call.
		result := eval.Apply(ctx, mainFn, []object.Object{}, mainPkg)

		// 5. Assert that no error occurred.
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("Evaluation failed with an unexpected error: %+v", err)
		}

		t.Log("Evaluation completed successfully without errors.")
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
