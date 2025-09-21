package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_StarExpr_PointerDeref(t *testing.T) {
	source := `
package main

type MyType struct{}

func (t *MyType) MyMethod() {}

func main() {
	p := &MyType{}
	(*p).MyMethod()
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var calledFunctions []object.Object

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		gopkg := pkgs[0]
		pkgEnv := object.NewEnclosedEnvironment(nil)
		evalpkg := &object.Package{
			Name:        gopkg.Name,
			Env:         pkgEnv,
			ScannedInfo: gopkg,
		}
		eval := New(s, s.Logger, nil, nil, WithPackages(map[string]*object.Package{
			gopkg.ImportPath: evalpkg,
		}))

		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0])
			}
			return nil
		})

		for _, file := range gopkg.AstFiles {
			eval.Eval(ctx, file, pkgEnv, gopkg)
		}

		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, gopkg, pkgEnv)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}

		if len(calledFunctions) == 0 {
			return fmt.Errorf("method call was not tracked")
		}

		// The call to `MyMethod` should be tracked.
		// Let's find it in the list of tracked calls.
		found := false
		for _, called := range calledFunctions {
			if fn, ok := called.(*object.Function); ok {
				if fn.Def != nil && fn.Def.Name == "MyMethod" {
					found = true
					break
				}
			}
		}

		if !found {
			return fmt.Errorf("expected 'MyMethod' to be called, but it was not found in tracked calls (found %d calls)", len(calledFunctions))
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
