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

const typeSwitchMethodSource = `
package main

// inspect is a special function that will be implemented as an intrinsic
// to check the value passed to it.
func inspect(s string) {}

type Greeter struct {
	Name string
}

func (g Greeter) Greet() {
	inspect(g.Name)
}

func main() {
	var i any = Greeter{Name: "World"}
	switch v := i.(type) {
	case Greeter:
		// This method call on the type-narrowed variable 'v' is what we want to test.
		// The evaluator should be able to resolve Greet() on the concrete type Greeter.
		v.Greet()
	case int:
		// This case is included to ensure the evaluator handles multiple branches correctly.
	}
}
`

func TestTypeSwitch_MethodCall(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": typeSwitchMethodSource,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedValue string
	var intrinsicCalled bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterIntrinsic("example.com/main.inspect", func(ctx context.Context, args ...object.Object) object.Object {
			intrinsicCalled = true
			if len(args) == 1 {
				if str, ok := args[0].(*object.String); ok {
					inspectedValue = str.Value
				}
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range mainPkg.AstFiles {
			if _, ok := eval.Eval(ctx, file, env, mainPkg).(*object.Error); ok {
				// Allow errors during initial scan
			}
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
			return fmt.Errorf("evaluation returned an unexpected error: %s", err.Message)
		}

		if !intrinsicCalled {
			return fmt.Errorf("expected intrinsic to be called, but it was not")
		}

		expectedValue := "World"
		if diff := cmp.Diff(expectedValue, inspectedValue); diff != "" {
			return fmt.Errorf("mismatch in inspected value (-want +got):\n%s", diff)
		}

		return nil
	}

	_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
	if err != nil {
		t.Fatalf("scantest.Run() failed unexpectedly: %v", err)
	}
}