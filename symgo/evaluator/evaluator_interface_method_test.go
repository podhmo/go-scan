package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ExternalInterfaceMethodCall(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"iface/iface.go": `
package iface
type Writer interface {
	Write(p []byte) (n int, err error)
}`,
		"main.go": `
package main
import "example.com/me/iface"
func Do(w iface.Writer) {
	w.Write(nil)
}
func main() {
	Do(nil)
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil)
		env := object.NewEnvironment()

		key := "(example.com/me/iface.Writer).Write"
		eval.RegisterIntrinsic(key, func(args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// Let scantest.Run create and configure the scanner.
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for external interface method was not called")
	}
}

func TestEval_InterfaceMethodCall(t *testing.T) {
	code := `
package main

type Writer interface {
	Write(p []byte) (n int, err error)
}

func Do(w Writer) {
	w.Write(nil)
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil)
		env := object.NewEnvironment()

		key := fmt.Sprintf("(%s.Writer).Write", pkg.ImportPath)
		eval.RegisterIntrinsic(key, func(args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// Let scantest.Run create and configure the scanner.
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for (main.Writer).Write was not called")
	}
}

func TestEval_InterfaceMethodCall_OnConcreteType(t *testing.T) {
	code := `
package main

type Speaker interface {
	Speak() string
}

type Dog struct {}
func (d *Dog) Speak() string { return "woof" }

func main() {
	var s Speaker
	s = &Dog{}
	s.Speak()
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var placeholderCalled bool
	var concreteFuncCalled bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil)
		env := object.NewEnvironment()

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			fnObj := args[0]
			switch fn := fnObj.(type) {
			case *object.SymbolicPlaceholder:
				if fn.UnderlyingMethod != nil && fn.UnderlyingMethod.Name == "Speak" {
					placeholderCalled = true
				}
			case *object.Function:
				// We want to ensure the concrete method is NOT called directly by the intrinsic system
				// when the variable's static type is an interface.
				if fn.Name.Name == "Speak" {
					concreteFuncCalled = true
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !placeholderCalled {
		t.Errorf("expected SymbolicPlaceholder to be created for interface call on concrete type, but it was not")
	}
	if concreteFuncCalled {
		t.Errorf("expected interface call to NOT resolve to concrete function in intrinsic, but it did")
	}
}

func TestEval_InterfaceMethodCall_AcrossControlFlow(t *testing.T) {
	code := `
package main

var someCondition bool // This will be symbolic

type Speaker interface {
	Speak() string
}

type Dog struct{}
func (d *Dog) Speak() string { return "woof" }

type Cat struct{}
func (c *Cat) Speak() string { return "meow" }

func main() {
	var s Speaker
	if someCondition {
		s = &Dog{}
	} else {
		s = &Cat{}
	}
	s.Speak()
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var speakPlaceholder *object.SymbolicPlaceholder

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil)
		env := object.NewEnvironment()

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			if p, ok := args[0].(*object.SymbolicPlaceholder); ok {
				if p.UnderlyingMethod != nil && p.UnderlyingMethod.Name == "Speak" {
					speakPlaceholder = p
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if speakPlaceholder == nil {
		t.Fatalf("SymbolicPlaceholder for Speak method was not captured")
	}

	if len(speakPlaceholder.PossibleConcreteTypes) != 2 {
		t.Errorf("expected 2 possible concrete types, but got %d", len(speakPlaceholder.PossibleConcreteTypes))
		for i, ft := range speakPlaceholder.PossibleConcreteTypes {
			t.Logf("  type %d: %s", i, ft.String())
		}
	}

	foundTypes := make(map[string]bool)
	for _, ft := range speakPlaceholder.PossibleConcreteTypes {
		var name string
		if ft.IsPointer {
			name = fmt.Sprintf("%s.*%s", ft.Elem.FullImportPath, ft.Elem.TypeName)
		} else {
			name = fmt.Sprintf("%s.%s", ft.FullImportPath, ft.TypeName)
		}
		foundTypes[name] = true
	}

	if !foundTypes["example.com/me.*Dog"] {
		t.Errorf("did not find *Dog in possible concrete types")
	}
	if !foundTypes["example.com/me.*Cat"] {
		t.Errorf("did not find *Cat in possible concrete types")
	}
}
