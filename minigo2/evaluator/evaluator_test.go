package evaluator_test

import (
	"go/parser"
	"testing"

	"github.com/podhmo/go-scan/minigo2/evaluator"
	"github.com/podhmo/go-scan/minigo2/object"
)

// testEval is a helper that parses the input string as a Go expression
// and runs the evaluator on the resulting AST node.
func testEval(input string) object.Object {
	expr, err := parser.ParseExpr(input)
	if err != nil {
		// This simplifies testing; we assume valid input for successful eval tests.
		// A test that expects a parse error would be structured differently.
		return nil
	}
	return evaluator.Eval(expr)
}

// testIntegerObject is a helper to check if an object is an Integer
// with the expected value.
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

func TestEvalIntegerExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"5", 5},
		{"10", 10},
		{"0", 0},
		{"999", 999},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(tt.input)
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
		{`"minigo"`, "minigo"},
		{`"hello world!"`, "hello world!"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(tt.input)
			testStringObject(t, evaluated, tt.expected)
		})
	}
}
