package evaluator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/scope"
)

func TestEval_StringLiteral(t *testing.T) {
	input := `"hello world"`
	expr, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %v", err)
	}

	eval := New()
	scope := scope.NewScope()
	obj := eval.Eval(expr, scope)

	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("Eval() returned wrong type. want=*object.String, got=%T (%+v)", obj, obj)
	}

	if str.Value != "hello world" {
		t.Errorf("String has wrong value. want=%q, got=%q", "hello world", str.Value)
	}
}

func TestEval_Identifier(t *testing.T) {
	input := `myVar`
	expr, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %v", err)
	}

	eval := New()
	s := scope.NewScope()
	expectedObj := &object.String{Value: "i am myVar"}
	s.Set("myVar", expectedObj)

	obj := eval.Eval(expr, s)

	if obj != expectedObj {
		t.Errorf("Eval() returned wrong object. want=%+v, got=%+v", expectedObj, obj)
	}
}

func TestEval_IdentifierNotFound(t *testing.T) {
	input := `nonExistent`
	expr, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %v", err)
	}

	eval := New()
	s := scope.NewScope()
	obj := eval.Eval(expr, s)

	errObj, ok := obj.(*object.Error)
	if !ok {
		t.Fatalf("Eval() did not return an error. got=%T (%+v)", obj, obj)
	}

	expectedMsg := "identifier not found: nonExistent"
	if errObj.Message != expectedMsg {
		t.Errorf("Error message wrong. want=%q, got=%q", expectedMsg, errObj.Message)
	}
}

func TestEval_AssignStmt(t *testing.T) {
	input := `x = "hello"`
	// Use parser.ParseFile to get a statement list, as assignment is a statement, not an expression.
	src := fmt.Sprintf("package main\n\nfunc main() {\n\t%s\n}", input)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile() failed: %v", err)
	}
	// Extract the assignment statement from the AST
	stmt := file.Decls[0].(*ast.FuncDecl).Body.List[0]

	eval := New()
	s := scope.NewScope()
	// s.Set("x", &object.String{Value: "initial"}) // Pre-declare the variable

	eval.Eval(stmt, s)

	// Check if the value was set correctly in the scope
	obj, ok := s.Get("x")
	if !ok {
		t.Fatalf("scope.Get() failed, 'x' not found")
	}

	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("scope contains wrong type for 'x'. want=*object.String, got=%T (%+v)", obj, obj)
	}

	if str.Value != "hello" {
		t.Errorf("String has wrong value. want=%q, got=%q", "hello", str.Value)
	}
}

func TestEval_ReturnStmt(t *testing.T) {
	input := `return "hello"`
	src := fmt.Sprintf("package main\n\nfunc main() {\n\t%s\n}", input)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile() failed: %v", err)
	}
	// We evaluate the whole function body
	block := file.Decls[0].(*ast.FuncDecl).Body

	eval := New()
	s := scope.NewScope()
	obj := eval.Eval(block, s)

	// The result of the block should be a ReturnValue
	retVal, ok := obj.(*object.ReturnValue)
	if !ok {
		t.Fatalf("Eval() did not return *object.ReturnValue. got=%T (%+v)", obj, obj)
	}

	// The wrapped value should be a String
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("ReturnValue has wrong type. want=*object.String, got=%T (%+v)", retVal.Value, retVal.Value)
	}

	if str.Value != "hello" {
		t.Errorf("String has wrong value. want=%q, got=%q", "hello", str.Value)
	}
}

func TestEval_UnsupportedNode(t *testing.T) {
	// Use a node type we haven't implemented yet
	node := &ast.ForStmt{
		For:  token.NoPos,
		Body: &ast.BlockStmt{},
	}

	eval := New()
	s := scope.NewScope()
	obj := eval.Eval(node, s)

	errObj, ok := obj.(*object.Error)
	if !ok {
		t.Fatalf("Eval() did not return an error for unsupported node. got=%T", obj)
	}

	expectedMsg := "evaluation not implemented for *ast.ForStmt"
	if errObj.Message != expectedMsg {
		t.Errorf("Error message wrong. want=%q, got=%q", expectedMsg, errObj.Message)
	}
}
