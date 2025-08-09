package minigo2

import (
	"context"
	"fmt"
	"testing"

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

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("x")
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
	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	// Register the "fmt" package with the Println function
	i.Register("fmt", map[string]any{
		"Println": fmt.Println,
	})

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("x")
	if !ok {
		t.Fatalf("variable 'x' not found in environment")
	}

	_, ok = val.(*object.Builtin)
	if !ok {
		t.Fatalf("x is not Builtin. got=%T (%+v)", val, val)
	}
}

func TestInterpreterEval_MultiFileImportAlias(t *testing.T) {
	fileA := `package main
import f "fmt"
var resultA = f.FmtFunc()
`
	fileB := `package main
import f "strings"
var resultB = f.StringsFunc()
`

	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	// Register mock functions that return unique strings
	i.Register("fmt", map[string]any{
		"FmtFunc": func() string { return "from fmt" },
	})
	i.Register("strings", map[string]any{
		"StringsFunc": func() string { return "from strings" },
	})

	// Load both files
	if err := i.LoadFile("file_a.go", []byte(fileA)); err != nil {
		t.Fatalf("LoadFile(A) failed: %v", err)
	}
	if err := i.LoadFile("file_b.go", []byte(fileB)); err != nil {
		t.Fatalf("LoadFile(B) failed: %v", err)
	}

	// Evaluate the loaded files
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check variable 'resultA'
	valA, okA := i.globalEnv.Get("resultA")
	if !okA {
		t.Fatalf("variable 'resultA' not found in environment")
	}
	strA, okA := valA.(*object.String)
	if !okA {
		t.Fatalf("resultA is not String. got=%T (%+v)", valA, valA)
	}
	if strA.Value != "from fmt" {
		t.Errorf("resultA has wrong value. got=%q, want=%q", strA.Value, "from fmt")
	}

	// Check variable 'resultB'
	valB, okB := i.globalEnv.Get("resultB")
	if !okB {
		t.Fatalf("variable 'resultB' not found in environment")
	}
	strB, okB := valB.(*object.String)
	if !okB {
		t.Fatalf("resultB is not String. got=%T (%+v)", valB, valB)
	}
	if strB.Value != "from strings" {
		t.Errorf("resultB has wrong value. got=%q, want=%q", strB.Value, "from strings")
	}
}

func TestInterpreterEval_SharedPackageInstance(t *testing.T) {
	fooFile := `package main
import x "sharedlib"
var valA = x.Get()
`
	barFile := `package main
import "sharedlib"
var valB = sharedlib.Get()
`

	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	// Register a mock package
	i.Register("sharedlib", map[string]any{
		"Get": func() int { return 42 },
	})

	// Load both files
	if err := i.LoadFile("foo.go", []byte(fooFile)); err != nil {
		t.Fatalf("LoadFile(foo) failed: %v", err)
	}
	if err := i.LoadFile("bar.go", []byte(barFile)); err != nil {
		t.Fatalf("LoadFile(bar) failed: %v", err)
	}

	// Evaluate
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check that both variables were set correctly
	valA, okA := i.globalEnv.Get("valA")
	if !okA {
		t.Fatal("variable 'valA' not found")
	}
	intA, okA := valA.(*object.Integer)
	if !okA || intA.Value != 42 {
		t.Errorf("valA has wrong value, got %v, want 42", valA)
	}

	valB, okB := i.globalEnv.Get("valB")
	if !okB {
		t.Fatal("variable 'valB' not found")
	}
	intB, okB := valB.(*object.Integer)
	if !okB || intB.Value != 42 {
		t.Errorf("valB has wrong value, got %v, want 42", valB)
	}

	// Check that the package instance was shared
	if len(i.packages) != 1 {
		t.Errorf("expected 1 cached package, but found %d", len(i.packages))
	}
	if _, ok := i.packages["sharedlib"]; !ok {
		t.Errorf("expected to find 'sharedlib' in package cache, but it was not there")
	}
}
