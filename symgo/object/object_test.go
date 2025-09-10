package object

import (
	"fmt"
	"go/ast"
	"testing"
)

func TestString_Type(t *testing.T) {
	s := &String{Value: "hello"}
	if s.Type() != STRING_OBJ {
		t.Errorf("s.Type() wrong. want=%q, got=%q", STRING_OBJ, s.Type())
	}
}

func TestString_Inspect(t *testing.T) {
	s := &String{Value: "hello world"}
	if s.Inspect() != `"hello world"` {
		t.Errorf(`s.Inspect() wrong. want=%q, got=%q`, `"hello world"`, s.Inspect())
	}
}

func TestFunction_Type(t *testing.T) {
	f := &Function{Name: &ast.Ident{Name: "myFunc"}}
	if f.Type() != FUNCTION_OBJ {
		t.Errorf("f.Type() wrong. want=%q, got=%q", FUNCTION_OBJ, f.Type())
	}
}

func TestFunction_Inspect(t *testing.T) {
	f := &Function{Name: &ast.Ident{Name: "myFunc"}}
	expected := "func myFunc() { ... }"
	if f.Inspect() != expected {
		t.Errorf("f.Inspect() wrong. want=%q, got=%q", expected, f.Inspect())
	}
}

func TestError_Type(t *testing.T) {
	e := &Error{Message: "an error"}
	if e.Type() != ERROR_OBJ {
		t.Errorf("e.Type() wrong. want=%q, got=%q", ERROR_OBJ, e.Type())
	}
}

func TestError_Inspect(t *testing.T) {
	e := &Error{Message: "file not found"}
	expected := "symgo runtime error: file not found\n"
	if e.Inspect() != expected {
		t.Errorf("e.Inspect() wrong. want=%q, got=%q", expected, e.Inspect())
	}
}

func TestSymbolicPlaceholder_Type(t *testing.T) {
	sp := &SymbolicPlaceholder{Reason: "test"}
	if sp.Type() != SYMBOLIC_OBJ {
		t.Errorf("sp.Type() wrong. want=%q, got=%q", SYMBOLIC_OBJ, sp.Type())
	}
}

func TestSymbolicPlaceholder_Inspect(t *testing.T) {
	reason := "result of external call"
	sp := &SymbolicPlaceholder{Reason: reason}
	expected := fmt.Sprintf("<Symbolic: %s>", reason)
	if sp.Inspect() != expected {
		t.Errorf("sp.Inspect() wrong. want=%q, got=%q", expected, sp.Inspect())
	}
}
