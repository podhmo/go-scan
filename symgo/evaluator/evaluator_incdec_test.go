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
			env := object.NewEnvironment()
			result := evaluator.Eval(context.Background(), mainFunc.Body, env, nil)

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
