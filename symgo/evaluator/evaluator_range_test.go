package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ForRangeStmt(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

func getItems() []string {
	println("getItems called")
	return []string{"a", "b", "c"}
}

func main() {
	for range getItems() {
		// do nothing
	}
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var getItemsCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]

		eval := New(s, s.Logger, nil, nil)

		// Register an intrinsic to track when getItems is called
		eval.RegisterIntrinsic(fmt.Sprintf("%s.getItems", pkg.ImportPath), func(ctx context.Context, args ...object.Object) object.Object {
			getItemsCalled = true
			return &object.Slice{} // Return a symbolic slice
		})

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			// This is expected if the package hasn't been evaluated yet.
			// Create a new environment for it.
			pkgEnv = object.NewEnclosedEnvironment(nil)
		}

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, pkgEnv, pkg)
		}

		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not an object.Function, got %T", mainFuncObj)
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, pkgEnv, token.NoPos)

		if !getItemsCalled {
			t.Error("expected getItems() to be called, but it was not")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
