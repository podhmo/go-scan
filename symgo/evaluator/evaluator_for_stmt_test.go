package evaluator

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvaluator_ForStmt_Cond(t *testing.T) {
	source := `
package main

var counter int

func check() bool {
	counter++
	return counter < 2
}

func main() {
	for check() {
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

		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
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
		result := eval.Apply(ctx, mainFunc, nil, mainPkg, pkgEnv)
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

func TestEvaluator_ForRangeStmt_FuncCall(t *testing.T) {
	source := `
package main

func getSlice() []int {
	return []int{1, 2, 3}
}

func main() {
	for _ = range getSlice() {
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

		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
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
		result := eval.Apply(ctx, mainFunc, nil, mainPkg, pkgEnv)
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
	// We only expect "getSlice", because the intrinsic is not called for the entrypoint ("main")
	want := []string{"getSlice"}
	if diff := cmp.Diff(want, discoveredFunctions); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
