package evaluator

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

const typeSwitchMultiCaseSource = `
package main

// Mover is an interface implemented by different types.
type Mover interface {
	Move()
}

// Person is one concrete type.
type Person struct {
	Name string
}
func (p Person) Move() {}
func (p Person) Greet() {
	// This method is NOT on the Mover interface.
	inspect("person:" + p.Name)
}

// Dog is another concrete type.
type Dog struct {
	Breed string
}
func (d Dog) Move() {}
func (d Dog) Bark() {
	// This method is also NOT on the Mover interface.
	inspect("dog:" + d.Breed)
}


// inspect is a special function that will be implemented as an intrinsic.
func inspect(s string) {}

func main() {
	var i Mover

	// First, test the Person case.
	i = Person{Name: "Alice"}
	switch v := i.(type) {
	case Person:
		v.Greet()
	case Dog:
		v.Bark() // This branch should not be taken for a Person.
	case nil:
		// do nothing
	}

	// Second, test the Dog case.
	i = Dog{Breed: "Retriever"}
	switch v := i.(type) {
	case Person:
		v.Greet() // This branch should not be taken for a Dog.
	case Dog:
		v.Bark()
	}
}
`

func TestTypeNarrowing_TypeSwitchMultiCase(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": typeSwitchMultiCaseSource,
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
			if res := eval.Eval(ctx, file, env, mainPkg); isError(res) {
				return res.(*object.Error)
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

	// Since the evaluator is an interpreter, it will only execute the matching case.
	// We expect to see one result from the Person switch, and one from the Dog switch.
	sort.Strings(inspectedValues)
	expected := []string{"dog:Retriever", "person:Alice"}
	if diff := cmp.Diff(expected, inspectedValues); diff != "" {
		t.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
	}
}