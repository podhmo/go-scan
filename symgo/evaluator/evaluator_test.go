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
	node := &ast.ChanType{
		Dir:   ast.SEND,
		Value: &ast.Ident{Name: "int"},
	}

	eval := New()
	s := scope.NewScope()
	obj := eval.Eval(node, s)

	errObj, ok := obj.(*object.Error)
	if !ok {
		t.Fatalf("Eval() did not return an error for unsupported node. got=%T", obj)
	}

	expectedMsg := "evaluation not implemented for *ast.ChanType"
	if errObj.Message != expectedMsg {
		t.Errorf("Error message wrong. want=%q, got=%q", expectedMsg, errObj.Message)
	}
}

func TestEval_IfStmt(t *testing.T) {
	// The evaluator should step into the if block regardless of the condition.
	input := `if true { x = "inside" }`
	stmt := parseStmt(t, input)

	eval := New()
	s := scope.NewScope()
	eval.Eval(stmt, s)

	obj, ok := s.Get("x")
	if !ok {
		t.Fatalf("x not found in scope")
	}
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("x is not a string, got %T", obj)
	}
	if str.Value != "inside" {
		t.Errorf("x has wrong value, want 'inside', got %q", str.Value)
	}
}

func TestEval_ForStmt(t *testing.T) {
	// The evaluator should evaluate the body once.
	input := `for i := 0; i < 10; i++ { y = "in-loop" }`
	stmt := parseStmt(t, input)

	eval := New()
	s := scope.NewScope()
	eval.Eval(stmt, s)

	obj, ok := s.Get("y")
	if !ok {
		t.Fatalf("y not found in scope")
	}
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("y is not a string, got %T", obj)
	}
	if str.Value != "in-loop" {
		t.Errorf("y has wrong value, want 'in-loop', got %q", str.Value)
	}
}

func TestEval_SwitchStmt(t *testing.T) {
	// The evaluator should evaluate all case blocks.
	input := `
switch "a" {
case "a":
	x = "is-a"
case "b":
	y = "is-b"
default:
	z = "is-default"
}
`
	stmt := parseStmt(t, input)

	eval := New()
	s := scope.NewScope()
	eval.Eval(stmt, s)

	// Check x
	objX, _ := s.Get("x")
	strX, ok := objX.(*object.String)
	if !ok || strX.Value != "is-a" {
		t.Errorf("x has wrong value, want 'is-a', got %v", objX)
	}

	// Check y
	objY, _ := s.Get("y")
	strY, ok := objY.(*object.String)
	if !ok || strY.Value != "is-b" {
		t.Errorf("y has wrong value, want 'is-b', got %v", objY)
	}

	// Check z
	objZ, _ := s.Get("z")
	strZ, ok := objZ.(*object.String)
	if !ok || strZ.Value != "is-default" {
		t.Errorf("z has wrong value, want 'is-default', got %v", objZ)
	}
}

// parseStmt is a helper to parse a single statement for testing.
func parseStmt(t *testing.T, input string) ast.Stmt {
	t.Helper()
	src := fmt.Sprintf("package main\n\nfunc main() {\n\t%s\n}", input)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile() failed for input\n%s\nerror: %v", input, err)
	}
	if len(file.Decls) == 0 || len(file.Decls[0].(*ast.FuncDecl).Body.List) == 0 {
		t.Fatalf("could not find statement in parsed file for input:\n%s", input)
	}
	return file.Decls[0].(*ast.FuncDecl).Body.List[0]
}
