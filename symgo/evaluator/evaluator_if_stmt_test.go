package evaluator_test

import (
	"context"
	"go/ast"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvaluator_IfStmt_Cond(t *testing.T) {
	source := `
package main

func check() bool {
	return true
}

func main() {
	if check() {
		// for side effect
	}
}
`
	// setup
	ctx := context.Background()
	files := map[string]string{
		"go.mod":  "module a.b/c",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// run
	var discoveredFunctions []string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
			if fn, ok := args[0].(*object.Function); ok {
				if fn.Name != nil {
					discoveredFunctions = append(discoveredFunctions, fn.Name.Name)
				}
			}
			return nil
		})

		mainPkg := pkgs[0]
		var mainFile *ast.File
		for _, f := range mainPkg.AstFiles {
			mainFile = f
			break
		}
		if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
			return err
		}

		mainFunc, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("main function not found")
		}
		// The call to Apply() starts the analysis from main. The default intrinsic
		// will only be called for functions *called by* main.
		_, err = interp.Apply(ctx, mainFunc, nil, mainPkg)
		return err
	}

	_, err := scantest.Run(t, ctx, dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("run failed: %+v", err)
	}

	// assert
	// We only expect "check", because the intrinsic is not called for the entrypoint ("main")
	want := []string{"check"}
	if diff := cmp.Diff(want, discoveredFunctions); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
