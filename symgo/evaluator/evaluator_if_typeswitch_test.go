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

func TestTypeSwitch_MethodCall(t *testing.T) {
	const typeSwitchMethodSource = `
package main

type Greeter struct { Name string }
func (g Greeter) Greet() { inspect(g.Name) }

// inspect is a special function that will be implemented as an intrinsic
func inspect(s string) {}

func main() {
	var i any = Greeter{Name: "World"}
	switch v := i.(type) {
	case Greeter:
		v.Greet() // This method call should be traced
	case int:
		// Other case
	}
}
`
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": typeSwitchMethodSource,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedValues []string

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
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}

		expected := []string{"World"}
		if diff := cmp.Diff(expected, inspectedValues); diff != "" {
			return fmt.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
		}

		return nil
	}

	_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestIfOk_FieldAccess(t *testing.T) {
	const ifOkFieldAccessSource = `
package main

func get_name() string { return "Alice" }
func inspect(s string) {} // Intrinsic

type User struct {
	Name string
}

func main() {
	var i any = User{Name: get_name()}
	if v, ok := i.(User); ok {
		inspect(v.Name) // This field access should be traced
	}
}
`
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": ifOkFieldAccessSource,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedValues []string
	var getNameCallCount int

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterIntrinsic("example.com/main.inspect", func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) == 1 {
				if str, ok := args[0].(*object.String); ok {
					inspectedValues = append(inspectedValues, str.Value)
				} else if placeholder, ok := args[0].(*object.SymbolicPlaceholder); ok {
					inspectedValues = append(inspectedValues, placeholder.Reason)
				}
			}
			return nil
		})
		eval.RegisterIntrinsic("example.com/main.get_name", func(ctx context.Context, args ...object.Object) object.Object {
			getNameCallCount++
			return &object.String{Value: "Alice"}
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
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}

		if getNameCallCount == 0 {
			return fmt.Errorf("get_name was never called")
		}

		expected := []string{"Alice"}
		if diff := cmp.Diff(expected, inspectedValues); diff != "" {
			return fmt.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
		}

		return nil
	}

	_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestTypeSwitch_Complex(t *testing.T) {
	const source = `
package main

// inspect is a special function that will be implemented as an intrinsic
func inspect(s string) {}

type I interface {
	DoI()
}

type A struct {
	Name string
}
func (a A) DoA() { inspect("A.DoA:" + a.Name) }
func (a A) DoI() { inspect("A.DoI:" + a.Name) }

type B struct {
	ID int
}
func (b B) DoB() { inspect("B.DoB") } // Note: No field access
func (b B) DoI() { inspect("B.DoI") }

func process(i I) {
	switch v := i.(type) {
	case A:
		v.DoA()
	case B:
		v.DoB()
	}
	// After the switch, the original interface method should still be callable.
	i.DoI()
}

func main() {
	a := A{Name: "alpha"}
	b := B{ID: 1}
	process(a)
	process(b)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedValues []string

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
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}

		expected := []string{
			"A.DoA:alpha",
			"A.DoI:alpha",
			"B.DoB",
			"B.DoI",
		}
		if diff := cmp.Diff(expected, inspectedValues); diff != "" {
			return fmt.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
		}

		return nil
	}

	_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
