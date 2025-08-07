package evaluator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/minigo2/object"
)

// testEval is a helper function to parse and evaluate a string of code.
func testEval(t *testing.T, input string) object.Object {
	t.Helper()
	// To parse statements, we need to wrap the input in a valid Go file structure.
	fullSource := fmt.Sprintf("package main\n\nfunc main() {\n%s\n}", input)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", fullSource, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse code: %v", err)
		return nil
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatalf("main function not found in parsed code")
		return nil
	}

	env := object.NewEnvironment()
	return Eval(mainFunc.Body, env)
}

// testIntegerObject is a helper to check if an object is an Integer with the expected value.
func testIntegerObject(t *testing.T, obj object.Object, expected int64) bool {
	t.Helper()

	result, ok := obj.(*object.Integer)
	if !ok {
		t.Errorf("object is not Integer. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%d, want=%d", result.Value, expected)
		return false
	}
	return true
}

// testStringObject is a helper to check if an object is a String
// with the expected value.
func testStringObject(t *testing.T, obj object.Object, expected string) bool {
	t.Helper()
	result, ok := obj.(*object.String)
	if !ok {
		t.Errorf("object is not String. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%q, want=%q", result.Value, expected)
		return false
	}
	return true
}

// testBooleanObject is a helper to check if an object is a Boolean
// with the expected value.
func testBooleanObject(t *testing.T, obj object.Object, expected bool) bool {
	t.Helper()
	result, ok := obj.(*object.Boolean)
	if !ok {
		t.Errorf("object is not Boolean. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%t, want=%t", result.Value, expected)
		return false
	}
	return true
}

// testNullObject is a helper to check if an object is Null.
func testNullObject(t *testing.T, obj object.Object) bool {
	t.Helper()
	if obj != nil && obj != object.NULL {
		// Note: Eval returns raw nil for some errors, which is what we check for.
		t.Errorf("object is not Null. got=%T (%+v)", obj, obj)
		return false
	}
	return true
}

func TestConstDeclarations(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or "nil" for expected failure
	}{
		{"const x = 10; x", int64(10)},
		{"const x = 10; const y = 20; y", int64(20)},
		{"const x = 10; var y = x; y", int64(10)},
		{"const x = 10; x = 20;", "nil"}, // Assignment failure
		{"const ( a = 1 ); a = 2", "nil"},   // Assignment failure in block
		{"const ( a = iota ); a", int64(0)},
		{"const ( a = iota; b ); b", int64(1)},
		{"const ( a = iota; b; c ); c", int64(2)},
		{"const ( a = 10; b; c ); c", int64(10)}, // Value carry-over
		{"const ( a = 10; b = 20; c ); c", int64(20)},
		{"const ( a = 1 << iota; b; c; d ); d", int64(8)}, // 1 << 3
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				if expected == "nil" {
					testNullObject(t, evaluated)
				} else {
					t.Fatalf("unsupported expected type for test: %T", expected)
				}
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestSwitchStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or "nil"
	}{
		// Basic switch with tag
		{"x := 2; switch x { case 1: 10; case 2: 20; case 3: 30; };", int64(20)},
		// Tag evaluation
		{"switch 1 + 1 { case 1: 10; case 2: 20; };", int64(20)},
		// Default case
		{"x := 4; switch x { case 1: 10; case 2: 20; default: 99; };", int64(99)},
		// No matching case, no default
		{"x := 4; switch x { case 1: 10; case 2: 20; };", "nil"},
		// Expressionless switch (switch true)
		{"x := 10; switch { case x > 5: 100; case x < 5: 200; };", int64(100)},
		{"x := 1; switch { case x > 5: 100; case x < 5: 200; };", int64(200)},
		// Case with multiple expressions
		{"x := 3; switch x { case 1, 2, 3: 30; default: 99; };", int64(30)},
		{"x := 4; switch x { case 1, 2, 3: 30; default: 99; };", int64(99)},
		// Switch with init statement
		{"switch x := 2; x { case 2: 20; };", int64(20)},
		// Init statement shadowing
		{"x := 10; switch x := 2; x { case 2: 20; }; x", int64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				if expected == "nil" {
					testNullObject(t, evaluated)
				} else {
					t.Fatalf("unsupported expected type for test: %T", expected)
				}
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestForStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{
			`
			var a = 0;
			for i := 0; i < 10; i = i + 1 {
				a = a + 1;
			}
			a;
			`,
			10,
		},
		{
			`
			var a = 0;
			for {
				a = a + 1;
				if a > 5 {
					break;
				}
			}
			a;
			`,
			6,
		},
		{
			`
			var a = 0;
			var i = 0;
			for i < 10 {
				i = i + 1;
				if i % 2 == 0 {
					continue;
				}
				a = a + 1;
			}
			a;
			`,
			5, // Only increments for odd numbers (1, 3, 5, 7, 9)
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestIfElseStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or "nil" for null
	}{
		{"if (true) { 10 }", int64(10)},
		{"if (false) { 10 }", "nil"},
		{"if (1) { 10 }", int64(10)}, // 1 is truthy
		{"if (1 < 2) { 10 }", int64(10)},
		{"if (1 > 2) { 10 }", "nil"},
		{"if (1 > 2) { 10 } else { 20 }", int64(20)},
		{"if (1 < 2) { 10 } else { 20 }", int64(10)},
		{"x := 10; if (x > 5) { 100 } else { 200 };", int64(100)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				if expected == "nil" {
					testNullObject(t, evaluated)
				} else {
					t.Fatalf("unsupported expected type for test: %T", expected)
				}
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestEvalBooleanExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"true == true", true},
		{"false == false", true},
		{"true == false", false},
		{"true != false", true},
		{"false != true", true},
		{"(1 < 2) == true", true},
		{"(1 > 2) == false", true},
		{"1 < 2", true},
		{"1 > 2", false},
		{"1 < 1", false},
		{"1 > 1", false},
		{"1 == 1", true},
		{"1 != 1", false},
		{"1 == 2", false},
		{"1 != 2", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testBooleanObject(t, evaluated, tt.expected)
		})
	}
}

func TestEvalIntegerExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"5", 5},
		{"10", 10},
		{"-5", -5},
		{"-10", -10},
		{"5 + 5 + 5 + 5 - 10", 10},
		{"2 * 2 * 2 * 2 * 2", 32},
		{"-50 + 100 + -50", 0},
		{"5 * 2 + 10", 20},
		{"5 + 2 * 10", 25},
		{"20 + 2 * -10", 0},
		{"50 / 2 * 2 + 10", 60},
		{"2 * (5 + 10)", 30},
		{"3 * 3 * 3 + 10", 37},
		{"3 * (3 * 3) + 10", 37},
		{"(5 + 10 * 2 + 15 / 3) * 2 + -10", 50},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestEvalStringExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`"hello" + " " + "world"`, "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testStringObject(t, evaluated, tt.expected)
		})
	}
}

func TestVarStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; a;", 5},
		{"var a = 5 * 5; a;", 25},
		{"var a = 5; var b = a; b;", 5},
		{"var a = 5; var b = a; var c = a + b + 5; c;", 15},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestAssignmentStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; a = 10; a;", 10},
		{"var a = 5; var b = 10; a = b; a;", 10},
		{"var a = 5; { a = 10; }; a;", 10}, // assignment affects outer scope
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestShortVarDeclarations(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"a := 5; a;", 5},
		{"a := 5 * 5; a;", 25},
		{"a := 5; b := a; b;", 5},
		{"a := 5; b := a; c := a + b + 5; c;", 15},
		{"a := 5; a = 10; a;", 10},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestLexicalScoping(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; { var a = 10; }; a;", 5}, // shadowing with var
		{"a := 5; { a := 10; }; a;", 5},       // shadowing with :=
		{"var a = 5; { var b = 10; }; a;", 5},
		{"var a = 5; { b := 10; }; a;", 5},
		{"a := 5; { b := a; }; a;", 5},
		{"var a = 1; { var a = 2; { var a = 3; }; a; }; a;", 1},
		{"var a = 1; { a = 2; { a = 3; }; a; }; a;", 3},         // nested assignment
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}
