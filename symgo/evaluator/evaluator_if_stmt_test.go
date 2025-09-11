package evaluator

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
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
	ctx := t.Context()
	files := map[string]string{
		"go.mod":  "module a.b/c",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// run
	var discoveredFunctions []string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if fn, ok := args[0].(*object.Function); ok {
				if fn.Name != nil {
					discoveredFunctions = append(discoveredFunctions, fn.Name.Name)
				}
			}
			return nil
		})

		mainPkg := pkgs[0]
		for _, f := range mainPkg.AstFiles {
			eval.Eval(ctx, f, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("a.b/c")
		if !ok {
			t.Fatal("could not get package env for 'a.b/c'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			t.Fatal("main function not found")
		}
		result := eval.Apply(ctx, mainFunc, nil, mainPkg)
		if err, ok := result.(*object.Error); ok {
			return err
		}
		return nil
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

func TestEvaluator_IfStmt_ResultIsNil(t *testing.T) {
	source := `
package main

func main() {
	x := 10
	if x > 5 {
		// This if statement is the last statement in the function.
		// Its result should not propagate up as the function's return value.
	}
}
`
	// setup
	ctx := t.Context()
	files := map[string]string{
		"go.mod":  "module a.b/c",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// run
	var finalResult object.Object
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		eval := New(s, s.Logger, nil, nil)
		mainPkg := pkgs[0]

		for _, f := range mainPkg.AstFiles {
			eval.Eval(ctx, f, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("a.b/c")
		if !ok {
			t.Fatal("could not get package env for 'a.b/c'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			t.Fatal("main function not found")
		}

		// The call to Apply() starts the analysis from main.
		result := eval.Apply(ctx, mainFunc, nil, mainPkg)
		if err, ok := result.(*object.Error); ok {
			return err
		}
		finalResult = result
		return nil
	}

	_, err := scantest.Run(t, ctx, dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("run failed: %+v", err)
	}

	// The result of a function that doesn't explicitly return should be NIL.
	// The bug is that `evalIfStmt` returns a placeholder, which then becomes
	// the return value of the whole function call.
	if unwrapped, ok := finalResult.(*object.ReturnValue); ok {
		finalResult = unwrapped.Value
	}
	if finalResult != object.NIL {
		t.Errorf("expected result to be NIL, but got %s (%T)", finalResult.Inspect(), finalResult)
	}
}
