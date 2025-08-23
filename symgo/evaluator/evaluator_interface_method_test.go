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
	if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
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
	if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
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

	if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !placeholderCalled {
		t.Errorf("expected SymbolicPlaceholder to be created for interface call on concrete type, but it was not")
	}
	if concreteFuncCalled {
		t.Errorf("expected interface call to NOT resolve to concrete function in intrinsic, but it did")
	}
}
