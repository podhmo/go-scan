package evaluator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo2/object"
)

// testEval is a helper function to parse and evaluate a string of code.
func testEval(t *testing.T, input string) object.Object {
	t.Helper()
	// To parse statements, we need to wrap the input in a valid Go file structure.
	fullSource := fmt.Sprintf("package main\n\nfunc main() {\n%s\n}", input)

	// Create a temporary file to hold the source code.
	tmpfile, err := os.CreateTemp("", "minigo2_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpfile.Name()) })

	if _, err := tmpfile.Write([]byte(fullSource)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, tmpfile.Name(), nil, parser.ParseComments)
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

	eval := New(fset)
	env := object.NewEnvironment()
	return eval.Eval(mainFunc.Body, env)
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

func testErrorObject(t *testing.T, obj object.Object, expectedMessage string) bool {
	t.Helper()
	errObj, ok := obj.(*object.Error)
	if !ok {
		t.Errorf("object is not Error. got=%T (%+v)", obj, obj)
		return false
	}
	if errObj.Message != expectedMessage {
		t.Errorf("wrong error message. expected=%q, got=%q", expectedMessage, errObj.Message)
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

func TestFunctionObject(t *testing.T) {
	input := "func(x int) { x + 2; };"
	evaluated := testEval(t, input)
	fn, ok := evaluated.(*object.Function)
	if !ok {
		t.Fatalf("object is not Function. got=%T (%+v)", evaluated, evaluated)
	}
	if len(fn.Parameters) != 1 {
		t.Fatalf("function has wrong parameters. Parameters=%+v", fn.Parameters)
	}
	if fn.Parameters[0].String() != "x" {
		t.Fatalf("parameter is not 'x'. got=%q", fn.Parameters[0])
	}
	// The exact string representation of the body is not critical to test here,
	// as long as we know it's a block statement.
	if fn.Body == nil {
		t.Fatalf("function body is nil")
	}
}

func TestFunctionApplication(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"identity := func(x int) { x; }; identity(5);", 5},
		{"identity := func(x int) { return x; }; identity(5);", 5},
		{"double := func(x int) { x * 2; }; double(5);", 10},
		{"add := func(x int, y int) { x + y; }; add(5, 5);", 10},
		{"add := func(x int, y int) { x + y; }; add(5 + 5, add(5, 5));", 20},
		{"func() { 5; }();", 5},
		{
			`
			newAdder := func(x int) {
				return func(y int) { x + y; };
			};
			addTwo := newAdder(2);
			addTwo(3);
			`,
			5,
		},
		{
			`
			fib := func(n int) {
				if (n < 2) {
					return n;
				}
				return fib(n-1) + fib(n-2);
			};
			fib(10);
			`,
			55,
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			testIntegerObject(t, testEval(t, tt.input), tt.expected)
		})
	}
}

func TestArrayLiterals(t *testing.T) {
	input := "[]int{1, 2 * 2, 3 + 3}"

	evaluated := testEval(t, input)
	result, ok := evaluated.(*object.Array)
	if !ok {
		t.Fatalf("object is not Array. got=%T (%+v)", evaluated, evaluated)
	}

	if len(result.Elements) != 3 {
		t.Fatalf("array has wrong num of elements. got=%d", len(result.Elements))
	}

	testIntegerObject(t, result.Elements[0], 1)
	testIntegerObject(t, result.Elements[1], 4)
	testIntegerObject(t, result.Elements[2], 6)
}

func TestMapLiterals(t *testing.T) {
	input := `
		map[string]int{
			"one": 10 - 9,
			"two": 2,
			"three": 6 / 2,
		}
	`
	evaluated := testEval(t, input)
	result, ok := evaluated.(*object.Map)
	if !ok {
		t.Fatalf("object is not Map. got=%T (%+v)", evaluated, evaluated)
	}

	expected := map[object.HashKey]int64{
		(&object.String{Value: "one"}).HashKey():   1,
		(&object.String{Value: "two"}).HashKey():   2,
		(&object.String{Value: "three"}).HashKey(): 3,
	}

	if len(result.Pairs) != len(expected) {
		t.Fatalf("map has wrong num of pairs. got=%d", len(result.Pairs))
	}

	for expectedKey, expectedValue := range expected {
		pair, ok := result.Pairs[expectedKey]
		if !ok {
			t.Errorf("no pair for given key in Pairs")
			continue
		}

		testIntegerObject(t, pair.Value, expectedValue)
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		input           string
		expected        any // string for single message, []string for multiple substrings
	}{
		{
			"5 + true;",
			"type mismatch: INTEGER + BOOLEAN",
		},
		{
			"5 + true; 5;",
			"type mismatch: INTEGER + BOOLEAN",
		},
		{
			"-true",
			"unknown operator: -BOOLEAN",
		},
		{
			"true + false;",
			"unknown operator: BOOLEAN + BOOLEAN",
		},
		{
			"5; true + false; 5",
			"unknown operator: BOOLEAN + BOOLEAN",
		},
		{
			"if (10 > 1) { true + false; }",
			"unknown operator: BOOLEAN + BOOLEAN",
		},
		{
			`
			if (10 > 1) {
				if (10 > 1) {
					return true + false;
				}
				return 1;
			}
			`,
			"unknown operator: BOOLEAN + BOOLEAN",
		},
		{
			"foobar",
			"identifier not found: foobar",
		},
		{
			`"Hello" - "World"`,
			"unknown operator: STRING - STRING",
		},
		{
			`x := 1; x(1)`,
			"not a function: INTEGER",
		},
		{
			`
			f := func(x int) {
				g := func() {
					x / 0;
				}
				g();
			}
			f(10);
			`,
			"division by zero",
		},
		{
			`
			bar := func() {
				"hello" - "world"
			};
			foo := func() {
				bar()
			};
			foo();
			`,
			[]string{
				"runtime error: unknown operator: STRING - STRING",
				"in bar",
				`"hello" - "world"`,
				"in foo",
				"bar()",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			errObj, ok := evaluated.(*object.Error)
			if !ok {
				t.Fatalf("no error object returned. got=%T(%+v)", evaluated, evaluated)
			}

			switch expected := tt.expected.(type) {
			case string:
				if errObj.Message != expected {
					t.Errorf("wrong error message. expected=%q, got=%q", expected, errObj.Message)
				}
			case []string:
				fullMessage := errObj.Inspect()
				for _, sub := range expected {
					if !strings.Contains(fullMessage, sub) {
						t.Errorf("expected error message to contain %q, but it did not.\nFull message:\n%s", sub, fullMessage)
					}
				}
			}
		})
	}
}

func TestConstDeclarations(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or error message string
	}{
		{"const x = 10; x", int64(10)},
		{"const x = 10; const y = 20; y", int64(20)},
		{"const x = 10; var y = x; y", int64(10)},
		{"const x = 10; x = 20;", "cannot assign to constant x"},
		{"const ( a = 1 ); a = 2", "cannot assign to constant a"},
		{"const ( a = iota ); a", int64(0)},
		{"const ( a = iota; b ); b", int64(1)},
		{"const ( a = iota; b; c ); c", int64(2)},
		{"const ( a = 10; b; c ); c", int64(10)},
		{"const ( a = 10; b = 20; c ); c", int64(20)},
		{"const ( a = 1 << iota; b; c; d ); d", int64(8)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
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

func TestStructs(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{
			`
			type Person struct {
				name string
				age int
			}
			p := Person{name: "Alice", age: 30}
			p.name
			`,
			"Alice",
		},
		{
			`
			type Point struct {
				x int
				y int
			}
			p := Point{x: 3, y: 5}
			p.x + p.y
			`,
			int64(8),
		},
		{
			`
			type User struct {
				active bool
				name string
			}
			u := User{name: "Bob", active: false}
			u.active = true
			u.active
			`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testStringObject(t, evaluated, expected)
			case bool:
				testBooleanObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
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
