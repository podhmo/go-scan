package evaluator

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalIncDecStmt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name: "increment",
			input: `
package main
func main() {
	x := 10
	x++
	return x
}`,
			expected: 11,
		},
		{
			name: "decrement",
			input: `
package main
func main() {
	x := 10
	x--
	return x
}`,
			expected: 9,
		},
		{
			name: "multiple increments and decrements",
			input: `
package main
func main() {
	x := 0
	x++
	x++
	x--
	x++
	return x
}`,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "main.go", tt.input, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}

			// Find the main function
			var mainFunc *ast.FuncDecl
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
					mainFunc = fn
					break
				}
			}
			if mainFunc == nil {
				t.Fatal("main function not found")
			}

			evaluator := New(nil, nil, nil, nil)
			env := object.NewEnclosedEnvironment(evaluator.UniverseEnv)
			result := evaluator.Eval(t.Context(), mainFunc.Body, env, nil)

			retVal, ok := result.(*object.ReturnValue)
			if !ok {
				t.Fatalf("expected a return value, but got %T: %v", result, result)
			}

			intVal, ok := retVal.Value.(*object.Integer)
			if !ok {
				t.Fatalf("expected an integer return value, but got %T", retVal.Value)
			}

			if intVal.Value != tt.expected {
				t.Errorf("expected return value %d, but got %d", tt.expected, intVal.Value)
			}
		})
	}
}

func TestEvalIncDecStmt_Symbolic(t *testing.T) {
	input := `
package main

func getSymbolic() int {
	return 0 // This will be treated as symbolic in the test
}

func main() {
	x := getSymbolic()
	x++
	// no return, we just want to ensure it doesn't panic
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", input, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("main function not found")
	}

	evaluator := New(nil, nil, nil, nil)
	env := object.NewEnclosedEnvironment(evaluator.UniverseEnv)

	// Pre-populate the environment with a symbolic value for `getSymbolic`
	env.Set("getSymbolic", &object.Intrinsic{
		Fn: func(ctx context.Context, args ...object.Object) object.Object {
			// This needs to return a ReturnValue containing the placeholder
			// to simulate a function call result.
			return &object.ReturnValue{Value: &object.SymbolicPlaceholder{Reason: "symbolic integer"}}
		},
	})

	// The important part is that this does not panic or return an error.
	result := evaluator.Eval(t.Context(), mainFunc.Body, env, nil)

	if result != nil && result.Type() == object.ERROR_OBJ {
		t.Errorf("expected no error, but got %v", result.(*object.Error).Message)
	}
}
