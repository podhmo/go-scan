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

const ifOkMethodCallSource = `
package main

type Walker interface {
	Walk()
}

type Person struct {
	Name string
}

func (p Person) Walk() {
	// This method is part of the interface.
}

func (p Person) Greet() {
	// This method is NOT part of the interface.
	inspect(p.Name)
}

// inspect is a special function that will be implemented as an intrinsic.
func inspect(s string) {}

func main() {
	var i Walker = Person{Name: "Alice"}
	if p, ok := i.(Person); ok {
		// After the type assertion, we should be able to call
		// a method that is not on the Walker interface.
		p.Greet()
	}
}
`

func TestTypeNarrowing_IfOkMethodCall(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": ifOkMethodCallSource,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedValues []string
	var evalErr error

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterIntrinsic("example.com/main.inspect", func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) == 1 {
				if str, ok := args[0].(*object.String); ok {
					inspectedValues = append(inspectedValues, str.Value)
				}
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, env, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest(mainPkg.ImportPath)
		if !ok {
			return fmt.Errorf("package env not found for %q", mainPkg.ImportPath)
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc := mainFuncObj.(*object.Function)

		result := eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		if err, ok := result.(*object.Error); ok && err != nil {
			evalErr = err
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if evalErr != nil {
		t.Fatalf("evaluation failed unexpectedly: %v", evalErr)
	}

	expected := []string{"Alice"}
	if diff := cmp.Diff(expected, inspectedValues); diff != "" {
		t.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
	}
}
