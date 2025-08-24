package evaluator

import (
	"context"
	"fmt"
	"go/ast"
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
		env := object.NewEnvironment()

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)

		// The variables are defined inside the function, so we need to evaluate the function
		// to populate its environment.
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		// The function's environment is where the variables are stored.
		fnEnv := mainFunc.Env
		x, ok := fnEnv.Get("x")
		if !ok {
			// It might be in the block scope's environment, let's check there.
			// This is a simplification; a real interpreter would have a more complex env chain.
			if body, ok := mainFunc.Body.List[0].(*ast.DeclStmt); ok {
				if valSpec, ok := body.Decl.(*ast.GenDecl).Specs[0].(*ast.ValueSpec); ok {
					if valSpec.Names[0].Name == "x" {
						// This is getting complicated, let's just check the function's direct env.
					}
				}
			}
		}

		// Let's re-evaluate the block to get the final env state
		blockEnv := object.NewEnclosedEnvironment(env)
		for _, stmt := range mainFunc.Body.List {
			eval.Eval(ctx, stmt, blockEnv, pkg)
		}

		x, ok = blockEnv.Get("x")
		if !ok {
			return fmt.Errorf("variable 'x' not found")
		}
		if diff := cmp.Diff(&object.Integer{Value: 10}, x.(*object.Variable).Value); diff != "" {
			return fmt.Errorf("variable 'x' mismatch (-want +got):\n%s", diff)
		}

		y, ok := blockEnv.Get("y")
		if !ok {
			return fmt.Errorf("variable 'y' not found")
		}
		if diff := cmp.Diff(&object.String{Value: "hello"}, y.(*object.Variable).Value); diff != "" {
			return fmt.Errorf("variable 'y' mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
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
		env := object.NewEnvironment()

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFunc, _ := env.Get("main")

		blockEnv := object.NewEnclosedEnvironment(env)
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
	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
