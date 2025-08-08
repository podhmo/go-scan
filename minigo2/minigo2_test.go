package minigo2

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/object"
)

func TestNewInterpreter(t *testing.T) {
	_, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}
}

func TestInterpreterEval_SimpleExpression(t *testing.T) {
	input := `package main
var x = 1 + 2`
	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	_, err = i.Eval(context.Background(), Options{
		Source:   []byte(input),
		Filename: "test.go",
	})
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.env.Get("x")
	if !ok {
		t.Fatalf("variable 'x' not found in environment")
	}

	integer, ok := val.(*object.Integer)
	if !ok {
		t.Fatalf("x is not Integer. got=%T (%+v)", val, val)
	}
	if integer.Value != 3 {
		t.Errorf("x should be 3. got=%d", integer.Value)
	}
}

func TestInterpreterEval_Import(t *testing.T) {
	input := `package main

import "fmt"

var x = fmt.Println
`
	i, err := NewInterpreter(goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	_, err = i.Eval(context.Background(), Options{
		Source:   []byte(input),
		Filename: "test.go",
	})
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.env.Get("x")
	if !ok {
		t.Fatalf("variable 'x' not found in environment")
	}

	_, ok = val.(*object.Builtin)
	if !ok {
		t.Fatalf("x is not Builtin. got=%T (%+v)", val, val)
	}
}
