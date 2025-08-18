package evaluator

import (
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
