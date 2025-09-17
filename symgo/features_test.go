package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestFeature_ErrorWithPosition(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func main() {
	x := undefined_variable
}`, // error is on line 4
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil && !strings.Contains(err.Error(), "undefined_variable") {
				return fmt.Errorf("initial eval of file %s failed unexpectedly: %w", file.Name.Name, err)
			}
		}

		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		_, evalErr := interp.Apply(ctx, mainFuncObj, []symgo.Object{}, pkg)
		if evalErr == nil {
			t.Fatal("expected an error, but got nil")
		}

		expectedPosition := "main.go:4:"
		expectedMessage := "identifier not found: undefined_variable"

		if !strings.Contains(evalErr.Error(), expectedPosition) {
			return fmt.Errorf("error message does not contain expected position\nwant_substr: %q\ngot:         %q", expectedPosition, evalErr.Error())
		}
		if !strings.Contains(evalErr.Error(), expectedMessage) {
			return fmt.Errorf("error message does not contain expected message\nwant_substr: %q\ngot:         %q", expectedMessage, evalErr.Error())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestFeature_CharLiteral(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func getChar() rune {
	return 'a'
}
`,
		},
		EntryPoint: "example.com/me.getChar",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}

		intVal, ok := r.ReturnValue.(*object.Integer)
		if !ok {
			t.Fatalf("expected captured value to be *object.Integer, but got %T", r.ReturnValue)
		}

		expected := int64('a')
		if intVal.Value != expected {
			t.Errorf("char literal value is wrong\nwant: %d\ngot:  %d", expected, intVal.Value)
		}
	}
	symgotest.Run(t, tc, action)
}

func TestSymgo_ReturnedFunctionClosure(t *testing.T) {
	var usedByReturnedFuncCalled bool

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/func-return\n\ngo 1.21\n",
			"main.go": `
package main
import "example.com/func-return/service"
func main() {
    handler := service.GetHandler()
    handler()
}`,
			"service/service.go": `
package service

func GetHandler() func() {
    return func() {
        usedByReturnedFunc()
    }
}
func usedByReturnedFunc() {}
`,
		},
		EntryPoint: "example.com/func-return.main",
		Options: []symgotest.Option{
			symgotest.WithDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				if len(args) == 0 {
					return object.NIL
				}

				var key string
				switch fn := args[0].(type) {
				case *symgo.Function:
					if fn.Def != nil && fn.Package != nil {
						key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Def.Name)
					}
				case *symgo.SymbolicPlaceholder:
					if fn.UnderlyingFunc != nil && fn.Package != nil {
						key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.UnderlyingFunc.Name)
					}
				}

				if key == "example.com/func-return/service.usedByReturnedFunc" {
					usedByReturnedFuncCalled = true
				}
				return object.NIL
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		if !usedByReturnedFuncCalled {
			t.Errorf("expected usedByReturnedFunc to be called, but it was not")
		}
	}

	symgotest.Run(t, tc, action)
}

func TestBuiltin_Panic(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func main() {
	panic("test message")
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return err
			}
		}

		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		_, evalErr := interp.Apply(ctx, mainFuncObj, []symgo.Object{}, pkg)
		if evalErr == nil {
			return fmt.Errorf("expected a panic error, but got nil")
		}

		expectedMsg := "panic: test message"
		if !strings.Contains(evalErr.Error(), expectedMsg) {
			return fmt.Errorf("error message mismatch\nwant_substr: %q\ngot:         %q", expectedMsg, evalErr.Error())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestMultiValueAssignment(t *testing.T) {
	source := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func twoReturns() (int, string) {
	return 42, "hello"
}

func main() {
	x, y := twoReturns()
	_ = x
	_ = y
}`,
	}

	t.Run("call function with multi-return", func(t *testing.T) {
		tc := symgotest.TestCase{
			Source:     source,
			EntryPoint: "example.com/me.twoReturns",
		}
		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("Execution failed unexpectedly: %v", r.Error)
			}
			multiRet, ok := r.ReturnValue.(*object.MultiReturn)
			if !ok {
				t.Fatalf("expected inner value to be MultiReturn, got %T", r.ReturnValue)
			}
			if len(multiRet.Values) != 2 {
				t.Fatalf("expected 2 return values, got %d", len(multiRet.Values))
			}
		}
		symgotest.Run(t, tc, action)
	})

	t.Run("assignment from multi-return", func(t *testing.T) {
		tc := symgotest.TestCase{
			Source:     source,
			EntryPoint: "example.com/me.main",
		}
		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("Apply(main) failed unexpectedly: %v", r.Error)
			}
		}
		symgotest.Run(t, tc, action)
	})
}

func TestFeature_IfElseEvaluation(t *testing.T) {
	var patternCalled bool
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func MyPattern() {}

func main() {
	x := 1
	if x > 0 {
		// do nothing
	} else {
		MyPattern()
	}
}`,
		},
		EntryPoint: "example.com/me.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("example.com/me.MyPattern", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				patternCalled = true
				return nil
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		if !patternCalled {
			t.Errorf("pattern in else block was not called")
		}
	}

	symgotest.Run(t, tc, action)
}

func TestFeature_SprintfIntrinsic(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main
import "fmt"
func run() string {
	name := "world"
	return fmt.Sprintf("hello %s %d", name, 42)
}`,
		},
		EntryPoint: "example.com/me.run",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}

		strVal, ok := r.ReturnValue.(*object.String)
		if !ok {
			t.Fatalf("expected result value to be *object.String, but got %T", r.ReturnValue)
		}

		expected := "hello world 42"
		if strVal.Value != expected {
			t.Errorf("Sprintf result is wrong\nwant: %q\ngot:  %q", expected, strVal.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
