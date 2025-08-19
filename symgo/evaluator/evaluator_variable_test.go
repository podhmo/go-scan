package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalCallExprOnIntrinsic_StringManipulation(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

import "strings"

func run() string {
	x := "foo"
	y := strings.ToUpper(x)
	return y
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var finalResult object.Object
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		eval.RegisterIntrinsic("strings.ToUpper", func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return newError("ToUpper expects 1 argument")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				// This will fail if my fix is not applied
				t.Errorf("expected arg to be *object.String, but got %T", args[0])
				return newError("argument to ToUpper must be a string, got %s", args[0].Type())
			}
			return &object.String{Value: strings.ToUpper(s.Value)}
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		runFuncObj, ok := env.Get("run")
		if !ok {
			return fmt.Errorf("run function not found")
		}
		runFunc := runFuncObj.(*object.Function)

		// applyFunction returns the result of the function call
		finalResult = eval.applyFunction(runFunc, []object.Object{}, pkg)

		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if finalResult == nil {
		t.Fatal("finalResult was not set")
	}

	result, ok := finalResult.(*object.String)
	if !ok {
		t.Fatalf("expected result to be *object.String, but got %T", finalResult)
	}

	if want := "FOO"; result.Value != want {
		t.Errorf("string manipulation result is wrong, want %q, got %q", want, result.Value)
	}
}

func TestEvalCallExprOnIntrinsic_WithVariable(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

func myintrinsic(s string) {}

func main() {
	x := "foo"
	myintrinsic(x)
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var got string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		eval.RegisterIntrinsic("example.com/me.myintrinsic", func(args ...object.Object) object.Object {
			if len(args) > 0 {
				// Without the fix, args[0] is *object.Variable.
				// We want it to be *object.String.
				if s, ok := args[0].(*object.String); ok {
					got = s.Value
				} else {
					// This will fail before the fix.
					t.Errorf("expected arg to be *object.String, but got %T", args[0])
				}
			}
			return &object.SymbolicPlaceholder{}
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(mainFunc, []object.Object{}, pkg)

		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if want := "foo"; got != want {
		t.Errorf("intrinsic not called correctly, want %q, got %q", want, got)
	}
}
