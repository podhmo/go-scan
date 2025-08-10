package minigo_test

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

func TestSpecialForm(t *testing.T) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	var receivedNode ast.Node
	interp.RegisterSpecial("assert_ast", func(ctx *object.BuiltinContext, pos token.Pos, args []ast.Expr) object.Object {
		if len(args) != 1 {
			return ctx.NewError(pos, "expected 1 argument, got %d", len(args))
		}
		receivedNode = args[0]
		return object.NIL
	})

	source := `
package main
func main() {
	assert_ast(1 + 2)
}`
	err = interp.LoadFile("test.go", []byte(source))
	if err != nil {
		t.Fatalf("failed to load file: %v", err)
	}

	_, err = interp.Eval(nil)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	if receivedNode == nil {
		t.Fatal("special form was not called")
	}

	if _, ok := receivedNode.(*ast.BinaryExpr); !ok {
		t.Errorf("expected special form to receive *ast.BinaryExpr, but got %T", receivedNode)
	}
}

func TestSpecialForm_Vs_RegularFunction(t *testing.T) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	// Register a special form
	interp.RegisterSpecial("is_ast", func(ctx *object.BuiltinContext, pos token.Pos, args []ast.Expr) object.Object {
		// It should receive an AST node
		if len(args) != 1 {
			return ctx.NewError(pos, "is_ast: expected 1 argument")
		}
		_, isIdent := args[0].(*ast.Ident)
		return &object.Boolean{Value: isIdent}
	})

	// Register a regular function via the environment
	regularFunc := &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			// It should receive an evaluated object
			if len(args) != 1 {
				return ctx.NewError(pos, "is_obj: expected 1 argument")
			}
			_, isInt := args[0].(*object.Integer)
			return &object.Boolean{Value: isInt}
		},
	}
	interp.GlobalEnvForTest().Set("is_obj", regularFunc)

	source := `
package main
var special_result bool
var regular_result bool
func main() {
	x := 10
	special_result = is_ast(x)
	regular_result = is_obj(x)
}`
	err = interp.LoadFile("test.go", []byte(source))
	if err != nil {
		t.Fatalf("failed to load file: %v", err)
	}

	_, err = interp.Eval(nil)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	// Check results
	specialResult, ok := interp.GlobalEnvForTest().Get("special_result")
	if !ok {
		t.Fatal("special_result not found in environment")
	}
	if val, ok := specialResult.(*object.Boolean); !ok || !val.Value {
		t.Errorf("expected is_ast(x) to be true (got ident), but was %v", specialResult.Inspect())
	}

	regularResult, ok := interp.GlobalEnvForTest().Get("regular_result")
	if !ok {
		t.Fatal("regular_result not found in environment")
	}
	if val, ok := regularResult.(*object.Boolean); !ok || !val.Value {
		t.Errorf("expected is_obj(x) to be true (got integer), but was %v", regularResult.Inspect())
	}
}

func TestSpecialForm_NotFound(t *testing.T) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}
	source := `
package main
func main() {
	non_existent_special_form(1)
}`
	err = interp.LoadFile("test.go", []byte(source))
	if err != nil {
		t.Fatalf("failed to load file: %v", err)
	}

	_, err = interp.Eval(nil)
	if err == nil {
		t.Fatal("expected error for undefined special form, but got nil")
	}
	expectedErr := `runtime error: identifier not found: non_existent_special_form`
	if !contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, but got %q", expectedErr, err.Error())
	}
}

// A helper function to check for substrings, as the error format might have extra details.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(expectedErr)] == substr
}
func TestSpecialForm_ArgumentPassing(t *testing.T) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	var receivedArgs []ast.Expr
	interp.RegisterSpecial("capture_args", func(ctx *object.BuiltinContext, pos token.Pos, args []ast.Expr) object.Object {
		receivedArgs = args
		return object.NIL
	})

	source := `
package main
func main() {
	y := 1
	capture_args(y, 2 + 3, "hello")
}`
	err = interp.LoadFile("test.go", []byte(source))
	if err != nil {
		t.Fatalf("failed to load file: %v", err)
	}
	_, err = interp.Eval(nil)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	if len(receivedArgs) != 3 {
		t.Fatalf("expected to capture 3 arguments, but got %d", len(receivedArgs))
	}

	if _, ok := receivedArgs[0].(*ast.Ident); !ok {
		t.Errorf("arg 1: expected *ast.Ident, got %T", receivedArgs[0])
	}
	if _, ok := receivedArgs[1].(*ast.BinaryExpr); !ok {
		t.Errorf("arg 2: expected *ast.BinaryExpr, got %T", receivedArgs[1])
	}
	if _, ok := receivedArgs[2].(*ast.BasicLit); !ok {
		t.Errorf("arg 3: expected *ast.BasicLit, got %T", receivedArgs[2])
	}
}

var expectedErr = `runtime error: identifier not found: non_existent_special_form`
