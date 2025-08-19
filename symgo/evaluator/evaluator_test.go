package evaluator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_StringLiteral(t *testing.T) {
	input := `"hello world"`
	expr, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %v", err)
	}

	eval := New(nil, nil) // No scanner needed for this test
	env := object.NewEnvironment()
	obj := eval.Eval(expr, env)

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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	expectedObj := &object.String{Value: "i am myVar"}
	env.Set("myVar", expectedObj)

	obj := eval.Eval(expr, env)

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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	obj := eval.Eval(expr, env)

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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	env.Set("x", &object.String{Value: "initial"}) // Pre-declare the variable

	eval.Eval(stmt, env)

	// Check if the value was set correctly in the scope
	obj, ok := env.Get("x")
	if !ok {
		t.Fatalf("env.Get() failed, 'x' not found")
	}

	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("env contains wrong type for 'x'. want=*object.String, got=%T (%+v)", obj, obj)
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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	obj := eval.Eval(block, env)

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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	obj := eval.Eval(node, env)

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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	env.Set("x", &object.String{Value: "outside"}) // Pre-declare
	eval.Eval(stmt, env)

	// The assignment happens in an enclosed scope, so the outer scope is unaffected.
	obj, _ := env.Get("x")
	if obj.(*object.String).Value != "outside" {
		t.Errorf("outer scope was affected by inner scope assignment")
	}
}

func TestEval_ForStmt(t *testing.T) {
	// The evaluator should evaluate the body once.
	input := `for i := 0; i < 10; i++ { y = "in-loop" }`
	stmt := parseStmt(t, input)

	eval := New(nil, nil)
	env := object.NewEnvironment()
	env.Set("y", &object.String{Value: "outside"}) // Pre-declare
	eval.Eval(stmt, env)

	// Like the if-statement, the assignment is in an inner scope.
	obj, _ := env.Get("y")
	if obj.(*object.String).Value != "outside" {
		t.Errorf("outer scope was affected by inner scope assignment")
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

	eval := New(nil, nil)
	env := object.NewEnvironment()
	// Pre-declare so we can check them.
	env.Set("x", &object.String{Value: "outside"})
	env.Set("y", &object.String{Value: "outside"})
	env.Set("z", &object.String{Value: "outside"})

	eval.Eval(stmt, env)

	// All assignments happen in inner scopes, so we can't check them here.
	// The outer scope should be unaffected.
	objX, _ := env.Get("x")
	if objX.(*object.String).Value != "outside" {
		t.Errorf("x in outer scope was modified")
	}
	objY, _ := env.Get("y")
	if objY.(*object.String).Value != "outside" {
		t.Errorf("y in outer scope was modified")
	}
	objZ, _ := env.Get("z")
	if objZ.(*object.String).Value != "outside" {
		t.Errorf("z in outer scope was modified")
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
