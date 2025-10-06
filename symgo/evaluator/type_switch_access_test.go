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

const typeSwitchTracerSource = `
package main

type Mover interface {
	Move()
}

type Person struct {
	Name string
}
func (p Person) Move() {}
func (p Person) Greet() {
	inspect("Person.Greet")
}

type Dog struct {
	Breed string
}
func (d Dog) Move() {}
func (d Dog) Bark() {
	inspect("Dog.Bark")
}

func inspect(s string) {}

func process(i Mover) {
	switch v := i.(type) {
	case Person:
		v.Greet()
	case Dog:
		v.Bark()
	case nil:
		// do nothing
	}
}

func main() {
	var p Mover
	process(p)
}
`

func TestTypeNarrowing_TypeSwitchTracerBehavior(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": typeSwitchTracerSource,
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

	sort.Strings(inspectedValues)
	expected := []string{"Dog.Bark", "Person.Greet"}
	if diff := cmp.Diff(expected, inspectedValues); diff != "" {
		t.Errorf("mismatch in inspected values (-want +got):\n%s", diff)
	}
}