package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalVarStatement_WithScantest(t *testing.T) {
	source := `
package main
func main() {
	var x = 10
	var y = "hello"
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("package env for example.com/me not found")
		}

		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not a function, got %T", mainFuncObj)
		}

		// The variables are defined inside the function, so we need to evaluate the function
		// to populate its environment.
		// The result of applyFunction is a new environment.
		fnEnv, err := eval.extendFunctionEnv(ctx, mainFunc, []object.Object{}, nil)
		if err != nil {
			return err
		}
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, fnEnv, token.NoPos)

		x, ok := fnEnv.Get("x")
		if !ok {
			return fmt.Errorf("variable 'x' not found")
		}
		if diff := cmp.Diff(&object.Integer{Value: 10}, x.(*object.Variable).Value); diff != "" {
			return fmt.Errorf("variable 'x' mismatch (-want +got):\n%s", diff)
		}

		y, ok := fnEnv.Get("y")
		if !ok {
			return fmt.Errorf("variable 'y' not found")
		}
		if diff := cmp.Diff(&object.String{Value: "hello"}, y.(*object.Variable).Value); diff != "" {
			return fmt.Errorf("variable 'y' mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalVariableReassignment(t *testing.T) {
	source := `
package main
func main() {
	var i = 10
	i = 20
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("package env for example.com/me not found")
		}

		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		blockEnv := object.NewEnclosedEnvironment(pkgEnv)
		for _, stmt := range mainFunc.(*object.Function).Body.List {
			eval.Eval(ctx, stmt, blockEnv, pkg)
		}

		i, ok := blockEnv.Get("i")
		if !ok {
			return fmt.Errorf("variable 'i' not found")
		}
		if diff := cmp.Diff(&object.Integer{Value: 20}, i.(*object.Variable).Value); diff != "" {
			return fmt.Errorf("variable 'i' mismatch after reassignment (-want +got):\n%s", diff)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
