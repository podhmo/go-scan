package evaluator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo/object"
)

// parse is a helper to quickly get an AST from a string of code.
func parse(t *testing.T, code string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse code: %v", err)
	}
	return f
}

func TestEval_FunctionCall(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Expected Inspect() output of the final expression
	}{
		{
			name: "simple identity function",
			input: `
package main

func identity(s string) string {
	return s
}

func main() {
	return identity("hello")
}
`,
			expected: "hello",
		},
		{
			name: "recursive call",
			input: `
package main

func main() {
	return myFunc("foo")
}

func myFunc(s string) string {
	return s
}
`,
			expected: "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			astFile := parse(t, tt.input)

			eval := New(nil)
			env := object.NewEnvironment()
			eval.Eval(astFile, env)

			mainFuncObj, ok := env.Get("main")
			if !ok {
				t.Fatal("main function not found in environment")
			}
			mainFunc, ok := mainFuncObj.(*object.Function)
			if !ok {
				t.Fatalf("main is not a function, got %T", mainFuncObj)
			}

			returnStmt, ok := mainFunc.Body.List[0].(*ast.ReturnStmt)
			if !ok {
				t.Fatalf("first statement in main is not a return, got %T", mainFunc.Body.List[0])
			}
			callExpr, ok := returnStmt.Results[0].(*ast.CallExpr)
			if !ok {
				t.Fatalf("return is not a call expression, got %T", returnStmt.Results[0])
			}
			result := eval.Eval(callExpr, mainFunc.Env)

			str, ok := result.(*object.String)
			if !ok {
				t.Errorf("result is not a String, got %T (%s)", result, result.Inspect())
				return
			}
			if str.Value != tt.expected {
				t.Errorf("result.Value = %q, want %q", str.Value, tt.expected)
			}
		})
	}
}

func TestEval_SymbolicFunctionCall(t *testing.T) {
	input := `
package main

import "fmt"

func main() {
	return fmt.Sprintf("hello %s", "world")
}
`
	astFile := parse(t, input)

	s, err := scan.New(scan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("scan.New() failed: %v", err)
	}
	internalScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("s.ScannerForSymgo() failed: %v", err)
	}

	eval := New(internalScanner)
	env := object.NewEnvironment()
	eval.Eval(astFile, env)

	mainFuncObj, _ := env.Get("main")
	mainFunc := mainFuncObj.(*object.Function)

	returnStmt := mainFunc.Body.List[0].(*ast.ReturnStmt)
	callExpr := returnStmt.Results[0].(*ast.CallExpr)
	result := eval.Eval(callExpr, mainFunc.Env)

	if _, ok := result.(*object.SymbolicPlaceholder); !ok {
		t.Errorf("expected SymbolicPlaceholder, got %T (%s)", result, result.Inspect())
	}
}

func TestEval_IntrinsicFunctionCall(t *testing.T) {
	input := `
package main

func main() {
	return my_intrinsic("wow")
}
`
	astFile := parse(t, input)

	eval := New(nil)

	// Register a custom intrinsic function.
	eval.intrinsics.Register("my_intrinsic", func(env *object.Environment, args ...object.Object) object.Object {
		if len(args) != 1 {
			return &object.Error{Message: "wrong number of arguments"}
		}
		s, ok := args[0].(*object.String)
		if !ok {
			return &object.Error{Message: "argument must be a string"}
		}
		return &object.String{Value: "intrinsic says: " + s.Value}
	})

	env := object.NewEnvironment()
	eval.Eval(astFile, env)

	// Manually add the intrinsic to the environment for the test.
	// In a real scenario, this would be handled by resolving `my_intrinsic`
	// to its registered object.
	intrinsicFn, _ := eval.intrinsics.Get("my_intrinsic")
	env.Set("my_intrinsic", &object.Intrinsic{Fn: intrinsicFn})

	mainFuncObj, _ := env.Get("main")
	mainFunc := mainFuncObj.(*object.Function)
	returnStmt := mainFunc.Body.List[0].(*ast.ReturnStmt)
	callExpr := returnStmt.Results[0].(*ast.CallExpr)

	result := eval.Eval(callExpr, env) // Evaluate in the top-level env where intrinsic is registered

	str, ok := result.(*object.String)
	if !ok {
		t.Fatalf("result is not a String, got %T (%s)", result, result.Inspect())
	}
	expected := "intrinsic says: wow"
	if str.Value != expected {
		t.Errorf("result.Value = %q, want %q", str.Value, expected)
	}
}
